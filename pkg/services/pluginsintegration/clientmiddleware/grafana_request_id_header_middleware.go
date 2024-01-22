package clientmiddleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"slices"

	"github.com/google/uuid"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/plugins"
	"github.com/grafana/grafana/pkg/services/signingkeys"
	"github.com/grafana/grafana/pkg/setting"
)

const GrafanaRequestID = "X-Grafana-Request-ID"
const GrafanaSignedRequestID = "X-Grafana-Signed-Request-ID"

// NewHostedGrafanaACHeaderMiddleware creates a new plugins.ClientMiddleware that will
// generate a random request ID, sign it using internal key and populate X-Grafana-Request-ID with the request ID
// and X-Grafana-Signed-Request-ID with signed request ID. We can then use this to verify that the request
// is coming from hosted Grafana and is not an external request. This is used for IP range access control.
func NewHostedGrafanaACHeaderMiddleware(cfg *setting.Cfg) plugins.ClientMiddleware {
	return plugins.ClientMiddlewareFunc(func(next plugins.Client) plugins.Client {
		return &HostedGrafanaACHeaderMiddleware{
			next: next,
			log:  log.New("ip_header_middleware"),
			cfg:  cfg,
		}
	})
}

type HostedGrafanaACHeaderMiddleware struct {
	next   plugins.Client
	keySvc signingkeys.Service
	log    log.Logger
	cfg    *setting.Cfg
}

func (m *HostedGrafanaACHeaderMiddleware) applyGrafanaRequestIDHeader(pCtx backend.PluginContext, h backend.ForwardHTTPHeaders) {
	// if request is not for a datasource, skip the middleware
	if h == nil || pCtx.DataSourceInstanceSettings == nil {
		return
	}

	// Check if the request is for a datasource that is allowed to have the header
	url := pCtx.DataSourceInstanceSettings.URL
	if !slices.Contains(m.cfg.IPRangeACAllowedURLs, url) {
		return
	}

	// Generate a new Grafana request ID and sign it with the secret key
	uid, err := uuid.NewRandom()
	if err != nil {
		m.log.Error("Failed to generate Grafana request ID", "error", err)
		return
	}
	grafanaRequestID := uid.String()

	hmac := hmac.New(sha256.New, []byte(m.cfg.IPRangeACSecretKey))
	if _, err := hmac.Write([]byte(grafanaRequestID)); err != nil {
		m.log.Error("Failed to sign IP range access control header", "error", err)
		return
	}
	signedGrafanaRequestID := hex.EncodeToString(hmac.Sum(nil))
	h.SetHTTPHeader(GrafanaSignedRequestID, signedGrafanaRequestID)
	h.SetHTTPHeader(GrafanaRequestID, grafanaRequestID)
}

func (m *HostedGrafanaACHeaderMiddleware) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	if req == nil {
		return m.next.QueryData(ctx, req)
	}

	m.applyGrafanaRequestIDHeader(req.PluginContext, req)

	return m.next.QueryData(ctx, req)
}

func (m *HostedGrafanaACHeaderMiddleware) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	if req == nil {
		return m.next.CallResource(ctx, req, sender)
	}

	m.applyGrafanaRequestIDHeader(req.PluginContext, req)

	return m.next.CallResource(ctx, req, sender)
}

func (m *HostedGrafanaACHeaderMiddleware) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	if req == nil {
		return m.next.CheckHealth(ctx, req)
	}

	m.applyGrafanaRequestIDHeader(req.PluginContext, req)

	return m.next.CheckHealth(ctx, req)
}

// TODO check what all of these do and whether we need to apply the header for all of them
func (m *HostedGrafanaACHeaderMiddleware) CollectMetrics(ctx context.Context, req *backend.CollectMetricsRequest) (*backend.CollectMetricsResult, error) {
	return m.next.CollectMetrics(ctx, req)
}

func (m *HostedGrafanaACHeaderMiddleware) SubscribeStream(ctx context.Context, req *backend.SubscribeStreamRequest) (*backend.SubscribeStreamResponse, error) {
	return m.next.SubscribeStream(ctx, req)
}

func (m *HostedGrafanaACHeaderMiddleware) PublishStream(ctx context.Context, req *backend.PublishStreamRequest) (*backend.PublishStreamResponse, error) {
	return m.next.PublishStream(ctx, req)
}

func (m *HostedGrafanaACHeaderMiddleware) RunStream(ctx context.Context, req *backend.RunStreamRequest, sender *backend.StreamSender) error {
	return m.next.RunStream(ctx, req, sender)
}
