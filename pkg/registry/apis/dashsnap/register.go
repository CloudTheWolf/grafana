package dashsnap

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	common "k8s.io/kube-openapi/pkg/common"
	"k8s.io/kube-openapi/pkg/spec3"

	dashsnap "github.com/grafana/grafana/pkg/apis/dashsnap/v0alpha1"
	"github.com/grafana/grafana/pkg/infra/appcontext"
	"github.com/grafana/grafana/pkg/infra/log"
	contextmodel "github.com/grafana/grafana/pkg/services/contexthandler/model"
	"github.com/grafana/grafana/pkg/services/dashboardsnapshots"
	"github.com/grafana/grafana/pkg/services/featuremgmt"
	grafanaapiserver "github.com/grafana/grafana/pkg/services/grafana-apiserver"
	"github.com/grafana/grafana/pkg/services/grafana-apiserver/endpoints/request"
	"github.com/grafana/grafana/pkg/services/grafana-apiserver/utils"
	"github.com/grafana/grafana/pkg/setting"
	"github.com/grafana/grafana/pkg/util"
	"github.com/grafana/grafana/pkg/web"
)

var _ grafanaapiserver.APIGroupBuilder = (*SnapshotsAPIBuilder)(nil)

var resourceInfo = dashsnap.DashboardSnapshotResourceInfo

// This is used just so wire has something unique to return
type SnapshotsAPIBuilder struct {
	service    dashboardsnapshots.Service
	namespacer request.NamespaceMapper
	options    sharingOptionsGetter
	gv         schema.GroupVersion
	logger     log.Logger
}

func NewSnapshotsAPIBuilder(
	p dashboardsnapshots.Service,
	cfg *setting.Cfg,
) *SnapshotsAPIBuilder {
	return &SnapshotsAPIBuilder{
		service:    p,
		options:    newSharingOptionsGetter(cfg),
		namespacer: request.GetNamespaceMapper(cfg),
		gv:         resourceInfo.GroupVersion(),
		logger:     log.New("snapshots::RawHandlers"),
	}
}

func RegisterAPIService(
	p dashboardsnapshots.Service,
	apiregistration grafanaapiserver.APIRegistrar,
	cfg *setting.Cfg,
	features featuremgmt.FeatureToggles,
) *SnapshotsAPIBuilder {
	if !features.IsEnabledGlobally(featuremgmt.FlagGrafanaAPIServerWithExperimentalAPIs) {
		return nil // skip registration unless opting into experimental apis
	}
	builder := NewSnapshotsAPIBuilder(p, cfg)
	apiregistration.RegisterAPI(builder)
	return builder
}

func (b *SnapshotsAPIBuilder) GetGroupVersion() schema.GroupVersion {
	return b.gv
}

func addKnownTypes(scheme *runtime.Scheme, gv schema.GroupVersion) {
	scheme.AddKnownTypes(gv,
		&dashsnap.DashboardSnapshot{},
		&dashsnap.DashboardSnapshotList{},
		&dashsnap.SharingOptions{},
		&dashsnap.SharingOptionsList{},
		&dashsnap.FullDashboardSnapshot{},
		&dashsnap.DashboardSnapshotWithDeleteKey{},
		&metav1.Status{},
	)
}

func (b *SnapshotsAPIBuilder) InstallSchema(scheme *runtime.Scheme) error {
	addKnownTypes(scheme, b.gv)

	// Link this version to the internal representation.
	// This is used for server-side-apply (PATCH), and avoids the error:
	//   "no kind is registered for the type"
	addKnownTypes(scheme, schema.GroupVersion{
		Group:   b.gv.Group,
		Version: runtime.APIVersionInternal,
	})

	// If multiple versions exist, then register conversions from zz_generated.conversion.go
	// if err := playlist.RegisterConversions(scheme); err != nil {
	//   return err
	// }
	metav1.AddToGroupVersion(scheme, b.gv)
	return scheme.SetVersionPriority(b.gv)
}

func (b *SnapshotsAPIBuilder) GetAPIGroupInfo(
	scheme *runtime.Scheme,
	codecs serializer.CodecFactory, // pointer?
	optsGetter generic.RESTOptionsGetter,
) (*genericapiserver.APIGroupInfo, error) {
	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(dashsnap.GROUP, scheme, metav1.ParameterCodec, codecs)
	storage := map[string]rest.Storage{}

	legacyStore := &legacyStorage{
		service:    b.service,
		namespacer: b.namespacer,
		options:    b.options,
	}
	legacyStore.tableConverter = utils.NewTableConverter(
		resourceInfo.GroupResource(),
		[]metav1.TableColumnDefinition{
			{Name: "Name", Type: "string", Format: "name"},
			{Name: "Title", Type: "string", Format: "string", Description: "The snapshot name"},
			{Name: "Created At", Type: "date"},
		},
		func(obj any) ([]interface{}, error) {
			m, ok := obj.(*dashsnap.DashboardSnapshot)
			if ok {
				return []interface{}{
					m.Name,
					m.Spec.Title,
					m.CreationTimestamp.UTC().Format(time.RFC3339),
				}, nil
			}
			return nil, fmt.Errorf("expected snapshot")
		},
	)
	storage[resourceInfo.StoragePath()] = legacyStore
	storage[resourceInfo.StoragePath("body")] = &subBodyREST{
		service:    b.service,
		namespacer: b.namespacer,
	}

	storage["options"] = &optionsStorage{
		getter:         b.options,
		tableConverter: legacyStore.tableConverter,
	}

	apiGroupInfo.VersionedResourcesStorageMap[dashsnap.VERSION] = storage
	return &apiGroupInfo, nil
}

func (b *SnapshotsAPIBuilder) GetOpenAPIDefinitions() common.GetOpenAPIDefinitions {
	return dashsnap.GetOpenAPIDefinitions
}

// Register additional routes with the server
func (b *SnapshotsAPIBuilder) GetAPIRoutes() *grafanaapiserver.APIRoutes {
	tags := []string{"Create and Delete (custom routes)"}
	return &grafanaapiserver.APIRoutes{
		Namespace: []grafanaapiserver.APIRouteHandler{
			{
				Path: "dashsnaps/create",
				Spec: &spec3.PathProps{
					Summary:     "an example at the root level",
					Description: "longer description here?",
					Post: &spec3.Operation{
						OperationProps: spec3.OperationProps{
							Tags: tags,
							RequestBody: &spec3.RequestBody{
								RequestBodyProps: spec3.RequestBodyProps{
									Description: "TODO???? can we get the request+response shapes here",
								},
							},
						},
					},
				},
				Handler: func(w http.ResponseWriter, r *http.Request) {
					user, err := appcontext.User(r.Context())
					if err != nil {
						w.WriteHeader(500)
						return
					}
					vars := mux.Vars(r)
					info, err := request.ParseNamespace(vars["namespace"])
					if err != nil {
						_, _ = w.Write([]byte("expected namespace"))
						w.WriteHeader(400)
						return
					}
					if info.OrgID != user.OrgID {
						_, _ = w.Write([]byte("org id mismatch"))
						w.WriteHeader(401)
						return
					}
					wrap := &contextmodel.ReqContext{
						Logger: b.logger,
						Context: &web.Context{
							Req:  r,
							Resp: web.NewResponseWriter(r.Method, w),
						},
						SignedInUser: user,
					}
					opts, err := b.options(info.Value)
					if err != nil {
						wrap.JsonApiErr(http.StatusBadRequest, "error getting options", err)
						return
					}

					// This also writes the response
					dashboardsnapshots.CreateDashboardSnapshot(wrap, opts.Spec, b.service)
				},
			},
			{
				Path: "dashsnaps/delete/{deleteKey}",
				Spec: &spec3.PathProps{
					Summary:     "an example at the root level",
					Description: "longer description here?",
					Delete: &spec3.Operation{
						OperationProps: spec3.OperationProps{
							Tags: tags,
						},
					},
				},
				Handler: func(w http.ResponseWriter, r *http.Request) {
					ctx := r.Context()
					vars := mux.Vars(r)
					key := vars["deleteKey"]

					err := dashboardsnapshots.DeleteWithKey(ctx, key, b.service)
					if err != nil {
						_, _ = w.Write([]byte("Failed to delete external dashboard"))
						w.WriteHeader(500)
						return
					}

					js, _ := json.Marshal(&util.DynMap{
						"message": "Snapshot deleted. It might take an hour before it's cleared from any CDN caches.",
					})
					_, _ = w.Write(js)
					w.WriteHeader(200)
				},
			},
		},
	}
}

func (b *SnapshotsAPIBuilder) GetAuthorizer() authorizer.Authorizer {
	// NEED to match, currently we may be more restrictive than the first clause
	//
	// https://github.com/grafana/grafana/blob/f63e43c113ac0cf8f78ed96ee2953874139bd2dc/pkg/middleware/auth.go#L203
	// func SnapshotPublicModeOrSignedIn(cfg *setting.Cfg) web.Handler {
	// 	return func(c *contextmodel.ReqContext) {
	// 		if cfg.SnapshotPublicMode {
	// 			return
	// 		}

	// 		if !c.IsSignedIn {
	// 			notAuthorized(c)
	// 			return
	// 		}
	// 	}
	// }

	return authorizer.AuthorizerFunc(
		func(ctx context.Context, attr authorizer.Attributes) (authorized authorizer.Decision, reason string, err error) {
			// TODO -- something more restrictive?
			return authorizer.DecisionAllow, "", err
		})
}
