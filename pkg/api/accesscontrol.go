package api

import (
	"context"
	"fmt"

	ac "github.com/grafana/grafana/pkg/services/accesscontrol"
	contextmodel "github.com/grafana/grafana/pkg/services/contexthandler/model"
	"github.com/grafana/grafana/pkg/services/dashboards"
	"github.com/grafana/grafana/pkg/services/datasources"
	"github.com/grafana/grafana/pkg/services/featuremgmt"
	"github.com/grafana/grafana/pkg/services/libraryelements"
	"github.com/grafana/grafana/pkg/services/org"
	"github.com/grafana/grafana/pkg/services/pluginsintegration/pluginaccesscontrol"
	"github.com/grafana/grafana/pkg/tsdb/grafanads"
)

// API related actions
const (
	ActionProvisioningReload = "provisioning:reload"
)

// API related scopes
var (
	ScopeProvisionersAll           = ac.Scope("provisioners", "*")
	ScopeProvisionersDashboards    = ac.Scope("provisioners", "dashboards")
	ScopeProvisionersPlugins       = ac.Scope("provisioners", "plugins")
	ScopeProvisionersDatasources   = ac.Scope("provisioners", "datasources")
	ScopeProvisionersNotifications = ac.Scope("provisioners", "notifications")
	ScopeProvisionersAlertRules    = ac.Scope("provisioners", "alerting")
)

// declareFixedRoles declares to the AccessControl service fixed roles and their
// grants to organization roles ("Viewer", "Editor", "Admin") or "Grafana Admin"
// that HTTPServer needs
func (hs *HTTPServer) declareFixedRoles() error {
	// Declare plugins roles
	if err := pluginaccesscontrol.DeclareRBACRoles(hs.accesscontrolService, hs.Cfg); err != nil {
		return err
	}

	provisioningWriterRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:provisioning:writer",
			DisplayName: "Writer",
			Description: "Reload provisioning.",
			Group:       "Provisioning",
			Permissions: []ac.Permission{
				{
					Action: ActionProvisioningReload,
					Scope:  ScopeProvisionersAll,
				},
			},
		},
		Grants: []string{ac.RoleGrafanaAdmin},
	}

	datasourcesExplorerRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:datasources:explorer",
			DisplayName: "Explorer",
			Description: "Enable the Explore feature. Data source permissions still apply; you can only query data sources for which you have query permissions.",
			Group:       "Data sources",
			Permissions: []ac.Permission{
				{
					Action: ac.ActionDatasourcesExplore,
				},
			},
		},
		Grants: []string{string(org.RoleEditor)},
	}

	if hs.Cfg.ViewersCanEdit {
		datasourcesExplorerRole.Grants = append(datasourcesExplorerRole.Grants, string(org.RoleViewer))
	}

	datasourcesReaderRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:datasources:reader",
			DisplayName: "Reader",
			Description: "Read and query all data sources.",
			Group:       "Data sources",
			Permissions: []ac.Permission{
				{
					Action: datasources.ActionRead,
					Scope:  datasources.ScopeAll,
				},
				{
					Action: datasources.ActionQuery,
					Scope:  datasources.ScopeAll,
				},
			},
		},
		Grants: []string{string(org.RoleAdmin)},
	}

	builtInDatasourceReader := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:datasources.builtin:reader",
			DisplayName: "Built in reader",
			Description: "Read and query Grafana's built in test data sources.",
			Group:       "Data sources",
			Permissions: []ac.Permission{
				{
					Action: datasources.ActionRead,
					Scope:  fmt.Sprintf("%s%s", datasources.ScopePrefix, grafanads.DatasourceUID),
				},
				{
					Action: datasources.ActionQuery,
					Scope:  fmt.Sprintf("%s%s", datasources.ScopePrefix, grafanads.DatasourceUID),
				},
			},
			Hidden: true,
		},
		Grants: []string{string(org.RoleViewer)},
	}

	// when running oss or enterprise without a license all users should be able to query data sources
	if !hs.License.FeatureEnabled("dspermissions.enforcement") {
		datasourcesReaderRole.Grants = []string{string(org.RoleViewer)}
	}

	datasourcesCreatorRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:datasources:creator",
			DisplayName: "Creator",
			Description: "Create data sources.",
			Group:       "Data sources",
			Permissions: []ac.Permission{
				{
					Action: datasources.ActionCreate,
				},
			},
		},
		Grants: []string{},
	}

	datasourcesWriterRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:datasources:writer",
			DisplayName: "Writer",
			Description: "Create, update, delete, read, or query data sources.",
			Group:       "Data sources",
			Permissions: ac.ConcatPermissions(datasourcesReaderRole.Role.Permissions, []ac.Permission{
				{
					Action: datasources.ActionWrite,
					Scope:  datasources.ScopeAll,
				},
				{
					Action: datasources.ActionCreate,
				},
				{
					Action: datasources.ActionDelete,
					Scope:  datasources.ScopeAll,
				},
			}),
		},
		Grants: []string{string(org.RoleAdmin)},
	}

	datasourcesIdReaderRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:datasources.id:reader",
			DisplayName: "Data source ID reader",
			Description: "Read the ID of a data source based on its name.",
			Group:       "Infrequently used",
			Permissions: []ac.Permission{
				{
					Action: datasources.ActionIDRead,
					Scope:  datasources.ScopeAll,
				},
			},
		},
		Grants: []string{string(org.RoleViewer)},
	}

	apikeyReaderRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:apikeys:reader",
			DisplayName: "Reader",
			Description: "Gives access to read api keys.",
			Group:       "API Keys",
			Permissions: []ac.Permission{
				{
					Action: ac.ActionAPIKeyRead,
					Scope:  ac.ScopeAPIKeysAll,
				},
			},
		},
		Grants: []string{string(org.RoleAdmin)},
	}

	apikeyWriterRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:apikeys:writer",
			DisplayName: "Writer",
			Description: "Gives access to add and delete api keys.",
			Group:       "API Keys",
			Permissions: ac.ConcatPermissions(apikeyReaderRole.Role.Permissions, []ac.Permission{
				{
					Action: ac.ActionAPIKeyCreate,
				},
				{
					Action: ac.ActionAPIKeyDelete,
					Scope:  ac.ScopeAPIKeysAll,
				},
			}),
		},
		Grants: []string{string(org.RoleAdmin)},
	}

	orgReaderRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:organization:reader",
			DisplayName: "Reader",
			Description: "Read an organization, such as its ID, name, address, or quotas.",
			Group:       "Organizations",
			Permissions: []ac.Permission{
				{Action: ac.ActionOrgsRead},
				{Action: ac.ActionOrgsQuotasRead},
			},
		},
		Grants: []string{string(org.RoleViewer), ac.RoleGrafanaAdmin},
	}

	orgWriterRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:organization:writer",
			DisplayName: "Writer",
			Description: "Read an organization, its quotas, or its preferences. Update organization properties, or its preferences.",
			Group:       "Organizations",
			Permissions: ac.ConcatPermissions(orgReaderRole.Role.Permissions, []ac.Permission{
				{Action: ac.ActionOrgsPreferencesRead},
				{Action: ac.ActionOrgsWrite},
				{Action: ac.ActionOrgsPreferencesWrite},
			}),
		},
		Grants: []string{string(org.RoleAdmin)},
	}

	orgMaintainerRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:organization:maintainer",
			DisplayName: "Maintainer",
			Description: "Create, read, write, or delete an organization. Read or write an organization's quotas. Needs to be assigned globally.",
			Group:       "Organizations",
			Permissions: ac.ConcatPermissions(orgReaderRole.Role.Permissions, []ac.Permission{
				{Action: ac.ActionOrgsCreate},
				{Action: ac.ActionOrgsWrite},
				{Action: ac.ActionOrgsDelete},
				{Action: ac.ActionOrgsQuotasWrite},
			}),
		},
		Grants: []string{string(ac.RoleGrafanaAdmin)},
	}

	teamCreatorGrants := []string{string(org.RoleAdmin)}
	if hs.Cfg.EditorsCanAdmin {
		teamCreatorGrants = append(teamCreatorGrants, string(org.RoleEditor))
	}
	teamsCreatorRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:teams:creator",
			DisplayName: "Creator",
			Description: "Create teams and read organisation users (required to manage the created teams).",
			Group:       "Teams",
			Permissions: []ac.Permission{
				{Action: ac.ActionTeamsCreate},
				{Action: ac.ActionOrgUsersRead, Scope: ac.ScopeUsersAll},
			},
		},
		Grants: teamCreatorGrants,
	}

	teamsReaderRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:teams:read",
			DisplayName: "Reader",
			Description: "List all teams.",
			Group:       "Teams",
			Permissions: []ac.Permission{
				{Action: ac.ActionTeamsRead, Scope: ac.ScopeTeamsAll},
			},
		},
		Grants: []string{},
	}

	teamsWriterRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:teams:writer",
			DisplayName: "Writer",
			Description: "Create, read, write, or delete a team as well as controlling team memberships.",
			Group:       "Teams",
			Permissions: []ac.Permission{
				{Action: ac.ActionTeamsCreate},
				{Action: ac.ActionTeamsDelete, Scope: ac.ScopeTeamsAll},
				{Action: ac.ActionTeamsPermissionsRead, Scope: ac.ScopeTeamsAll},
				{Action: ac.ActionTeamsPermissionsWrite, Scope: ac.ScopeTeamsAll},
				{Action: ac.ActionTeamsRead, Scope: ac.ScopeTeamsAll},
				{Action: ac.ActionTeamsWrite, Scope: ac.ScopeTeamsAll},
			},
		},
		Grants: []string{string(org.RoleAdmin)},
	}

	annotationsReaderRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:annotations:reader",
			DisplayName: "Reader",
			Description: "Read annotations and tags",
			Group:       "Annotations",
			Permissions: []ac.Permission{
				{Action: ac.ActionAnnotationsRead, Scope: ac.ScopeAnnotationsAll},
			},
		},
		Grants: []string{string(org.RoleViewer)},
	}

	// TODO this role can be removed once we have rolled out FlagAnnotationPermissionUpdate to all users
	// keeping it in for now for backwards compatibility
	dashboardAnnotationsWriterRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:annotations.dashboard:writer",
			DisplayName: "Dashboard annotation writer",
			Description: "Update annotations associated with dashboards.",
			Group:       "Annotations",
			Permissions: []ac.Permission{
				{Action: ac.ActionAnnotationsCreate, Scope: ac.ScopeAnnotationsTypeDashboard},
				{Action: ac.ActionAnnotationsDelete, Scope: ac.ScopeAnnotationsTypeDashboard},
				{Action: ac.ActionAnnotationsWrite, Scope: ac.ScopeAnnotationsTypeDashboard},
			},
		},
		Grants: []string{string(org.RoleViewer)},
	}

	annotationsWriterRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:annotations:writer",
			DisplayName: "Writer",
			Description: "Update all annotations.",
			Group:       "Annotations",
			Permissions: []ac.Permission{
				{Action: ac.ActionAnnotationsCreate, Scope: ac.ScopeAnnotationsAll},
				{Action: ac.ActionAnnotationsDelete, Scope: ac.ScopeAnnotationsAll},
				{Action: ac.ActionAnnotationsWrite, Scope: ac.ScopeAnnotationsAll},
			},
		},
		Grants: []string{string(org.RoleEditor)},
	}

	if hs.Features.IsEnabled(context.Background(), featuremgmt.FlagAnnotationPermissionUpdate) {
		// Keeping the name to avoid breaking changes (for users who have assigned this role to grant permissions on organization annotations)
		annotationsReaderRole = ac.RoleRegistration{
			Role: ac.RoleDTO{
				Name:        "fixed:annotations:reader",
				DisplayName: "Organization annotation reader",
				Description: "Read organization annotations and annotation tags",
				Group:       "Annotations",
				Permissions: []ac.Permission{
					// Need to leave the permissions as they are, so that the seeder doesn't replace permissions when they have been removed from the basic role by the user
					// Otherwise we could split this into ac.ScopeAnnotationsTypeOrganization and ac.ScopeAnnotationsTypeDashboard scopes and eventually remove the dashboard scope.
					// https://github.com/grafana/identity-access-team/issues/524
					{Action: ac.ActionAnnotationsRead, Scope: ac.ScopeAnnotationsAll},
				},
			},
			Grants: []string{string(org.RoleViewer)},
		}

		// Keeping the name to avoid breaking changes (for users who have assigned this role to grant permissions on organization annotations)
		annotationsWriterRole = ac.RoleRegistration{
			Role: ac.RoleDTO{
				Name:        "fixed:annotations:writer",
				DisplayName: "Organization annotation writer",
				Description: "Update organization annotations.",
				Group:       "Annotations",
				Permissions: []ac.Permission{
					// Need to leave the permissions as they are, so that the seeder doesn't replace permissions when they have been removed from the basic role by the user
					// Otherwise we could split this into ac.ScopeAnnotationsTypeOrganization and ac.ScopeAnnotationsTypeDashboard scopes and eventually remove the dashboard scope.
					// https://github.com/grafana/identity-access-team/issues/524
					{Action: ac.ActionAnnotationsCreate, Scope: ac.ScopeAnnotationsAll},
					{Action: ac.ActionAnnotationsDelete, Scope: ac.ScopeAnnotationsAll},
					{Action: ac.ActionAnnotationsWrite, Scope: ac.ScopeAnnotationsAll},
				},
			},
			Grants: []string{string(org.RoleEditor)},
		}
	}

	dashboardsCreatorRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:dashboards:creator",
			DisplayName: "Creator",
			Description: "Create dashboard in general folder.",
			Group:       "Dashboards",
			Permissions: []ac.Permission{
				{Action: dashboards.ActionFoldersRead, Scope: dashboards.ScopeFoldersProvider.GetResourceScopeUID(ac.GeneralFolderUID)},
				{Action: dashboards.ActionDashboardsCreate, Scope: dashboards.ScopeFoldersProvider.GetResourceScopeUID(ac.GeneralFolderUID)},
			},
		},
		Grants: []string{"Editor"},
	}

	dashboardsReaderRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:dashboards:reader",
			DisplayName: "Reader",
			Description: "Read all dashboards.",
			Group:       "Dashboards",
			Permissions: []ac.Permission{
				{Action: dashboards.ActionDashboardsRead, Scope: dashboards.ScopeDashboardsAll},
			},
		},
		Grants: []string{"Admin"},
	}

	dashboardsWriterRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:dashboards:writer",
			DisplayName: "Writer",
			Group:       "Dashboards",
			Description: "Create, read, write or delete all dashboards and their permissions.",
			Permissions: ac.ConcatPermissions(dashboardsReaderRole.Role.Permissions, []ac.Permission{
				{Action: dashboards.ActionDashboardsWrite, Scope: dashboards.ScopeDashboardsAll},
				{Action: dashboards.ActionDashboardsDelete, Scope: dashboards.ScopeDashboardsAll},
				{Action: dashboards.ActionDashboardsCreate, Scope: dashboards.ScopeFoldersAll},
				{Action: dashboards.ActionDashboardsPermissionsRead, Scope: dashboards.ScopeDashboardsAll},
				{Action: dashboards.ActionDashboardsPermissionsWrite, Scope: dashboards.ScopeDashboardsAll},
			}),
		},
		Grants: []string{"Admin"},
	}

	foldersCreatorRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:folders:creator",
			DisplayName: "Creator",
			Description: "Create folders.",
			Group:       "Folders",
			Permissions: []ac.Permission{
				{Action: dashboards.ActionFoldersCreate},
			},
		},
		Grants: []string{"Editor"},
	}

	foldersReaderRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:folders:reader",
			DisplayName: "Reader",
			Description: "Read all folders and dashboards.",
			Group:       "Folders",
			Permissions: []ac.Permission{
				{Action: dashboards.ActionFoldersRead, Scope: dashboards.ScopeFoldersAll},
				{Action: dashboards.ActionDashboardsRead, Scope: dashboards.ScopeFoldersAll},
			},
		},
		Grants: []string{"Admin"},
	}

	foldersWriterRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:folders:writer",
			DisplayName: "Writer",
			Description: "Create, read, write or delete all folders and dashboards and their permissions.",
			Group:       "Folders",
			Permissions: ac.ConcatPermissions(
				foldersReaderRole.Role.Permissions,
				[]ac.Permission{
					{Action: dashboards.ActionFoldersCreate},
					{Action: dashboards.ActionFoldersWrite, Scope: dashboards.ScopeFoldersAll},
					{Action: dashboards.ActionFoldersDelete, Scope: dashboards.ScopeFoldersAll},
					{Action: dashboards.ActionDashboardsWrite, Scope: dashboards.ScopeFoldersAll},
					{Action: dashboards.ActionDashboardsDelete, Scope: dashboards.ScopeFoldersAll},
					{Action: dashboards.ActionDashboardsCreate, Scope: dashboards.ScopeFoldersAll},
					{Action: dashboards.ActionDashboardsPermissionsRead, Scope: dashboards.ScopeFoldersAll},
					{Action: dashboards.ActionDashboardsPermissionsWrite, Scope: dashboards.ScopeFoldersAll},
				}),
		},
		Grants: []string{"Admin"},
	}

	libraryPanelsCreatorRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:library.panels:creator",
			DisplayName: "Creator",
			Description: "Create library panel in general folder.",
			Group:       "Library panels",
			Permissions: []ac.Permission{
				{Action: dashboards.ActionFoldersRead, Scope: dashboards.ScopeFoldersProvider.GetResourceScopeUID(ac.GeneralFolderUID)},
				{Action: libraryelements.ActionLibraryPanelsCreate, Scope: dashboards.ScopeFoldersProvider.GetResourceScopeUID(ac.GeneralFolderUID)},
			},
		},
		Grants: []string{"Editor"},
	}

	libraryPanelsReaderRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:library.panels:reader",
			DisplayName: "Reader",
			Description: "Read all library panels.",
			Group:       "Library panels",
			Permissions: []ac.Permission{
				{Action: libraryelements.ActionLibraryPanelsRead, Scope: dashboards.ScopeFoldersAll},
			},
		},
		Grants: []string{"Admin"},
	}

	libraryPanelsGeneralReaderRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:library.panels:general.reader",
			DisplayName: "General reader",
			Description: "Read all library panels in general folder.",
			Group:       "Library panels",
			Permissions: []ac.Permission{
				{Action: libraryelements.ActionLibraryPanelsRead, Scope: dashboards.ScopeFoldersProvider.GetResourceScopeUID(ac.GeneralFolderUID)},
			},
		},
		Grants: []string{"Viewer"},
	}

	libraryPanelsWriterRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:library.panels:writer",
			DisplayName: "Writer",
			Group:       "Library panels",
			Description: "Create, read, write or delete all library panels and their permissions.",
			Permissions: ac.ConcatPermissions(libraryPanelsReaderRole.Role.Permissions, []ac.Permission{
				{Action: libraryelements.ActionLibraryPanelsWrite, Scope: dashboards.ScopeFoldersAll},
				{Action: libraryelements.ActionLibraryPanelsDelete, Scope: dashboards.ScopeFoldersAll},
				{Action: libraryelements.ActionLibraryPanelsCreate, Scope: dashboards.ScopeFoldersAll},
			}),
		},
		Grants: []string{"Admin"},
	}

	libraryPanelsGeneralWriterRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:library.panels:general.writer",
			DisplayName: "General writer",
			Group:       "Library panels",
			Description: "Create, read, write or delete all library panels and their permissions in the general folder.",
			Permissions: ac.ConcatPermissions(libraryPanelsGeneralReaderRole.Role.Permissions, []ac.Permission{
				{Action: libraryelements.ActionLibraryPanelsWrite, Scope: dashboards.ScopeFoldersProvider.GetResourceScopeUID(ac.GeneralFolderUID)},
				{Action: libraryelements.ActionLibraryPanelsDelete, Scope: dashboards.ScopeFoldersProvider.GetResourceScopeUID(ac.GeneralFolderUID)},
				{Action: libraryelements.ActionLibraryPanelsCreate, Scope: dashboards.ScopeFoldersProvider.GetResourceScopeUID(ac.GeneralFolderUID)},
			}),
		},
		Grants: []string{"Editor"},
	}

	publicDashboardsWriterRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:dashboards.public:writer",
			DisplayName: "Public Dashboard writer",
			Description: "Create, write or disable a public dashboard.",
			Group:       "Dashboards",
			Permissions: []ac.Permission{
				{Action: dashboards.ActionDashboardsPublicWrite, Scope: dashboards.ScopeDashboardsAll},
			},
		},
		Grants: []string{"Admin"},
	}

	featuremgmtReaderRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:featuremgmt:reader",
			DisplayName: "Reader",
			Description: "Read feature toggles",
			Group:       "Feature Management",
			Permissions: []ac.Permission{
				{Action: ac.ActionFeatureManagementRead},
			},
		},
		Grants: []string{"Admin"},
	}

	featuremgmtWriterRole := ac.RoleRegistration{
		Role: ac.RoleDTO{
			Name:        "fixed:featuremgmt:writer",
			DisplayName: "Writer",
			Description: "Write feature toggles",
			Group:       "Feature Management",
			Permissions: []ac.Permission{
				{Action: ac.ActionFeatureManagementWrite},
			},
		},
		Grants: []string{"Admin"},
	}

	roles := []ac.RoleRegistration{provisioningWriterRole, datasourcesReaderRole, builtInDatasourceReader, datasourcesWriterRole,
		datasourcesIdReaderRole, datasourcesCreatorRole, orgReaderRole, orgWriterRole,
		orgMaintainerRole, teamsCreatorRole, teamsWriterRole, teamsReaderRole, datasourcesExplorerRole,
		annotationsReaderRole, dashboardAnnotationsWriterRole, annotationsWriterRole,
		dashboardsCreatorRole, dashboardsReaderRole, dashboardsWriterRole,
		foldersCreatorRole, foldersReaderRole, foldersWriterRole, apikeyReaderRole, apikeyWriterRole,
		publicDashboardsWriterRole, featuremgmtReaderRole, featuremgmtWriterRole, libraryPanelsCreatorRole,
		libraryPanelsReaderRole, libraryPanelsWriterRole, libraryPanelsGeneralReaderRole, libraryPanelsGeneralWriterRole}

	if hs.Features.IsEnabled(context.Background(), featuremgmt.FlagAnnotationPermissionUpdate) {
		allAnnotationsReaderRole := ac.RoleRegistration{
			Role: ac.RoleDTO{
				Name:        "fixed:annotations.all:reader",
				DisplayName: "Reader",
				Description: "Read all annotations and tags",
				Group:       "Annotations",
				Permissions: []ac.Permission{
					{Action: ac.ActionAnnotationsRead, Scope: ac.ScopeAnnotationsTypeOrganization},
					{Action: ac.ActionAnnotationsRead, Scope: dashboards.ScopeFoldersAll},
				},
			},
			Grants: []string{string(org.RoleAdmin)},
		}

		allAnnotationsWriterRole := ac.RoleRegistration{
			Role: ac.RoleDTO{
				Name:        "fixed:annotations.all:writer",
				DisplayName: "Writer",
				Description: "Update all annotations.",
				Group:       "Annotations",
				Permissions: []ac.Permission{
					{Action: ac.ActionAnnotationsCreate, Scope: ac.ScopeAnnotationsTypeOrganization},
					{Action: ac.ActionAnnotationsCreate, Scope: dashboards.ScopeFoldersAll},
					{Action: ac.ActionAnnotationsDelete, Scope: ac.ScopeAnnotationsTypeOrganization},
					{Action: ac.ActionAnnotationsDelete, Scope: dashboards.ScopeFoldersAll},
					{Action: ac.ActionAnnotationsWrite, Scope: ac.ScopeAnnotationsTypeOrganization},
					{Action: ac.ActionAnnotationsWrite, Scope: dashboards.ScopeFoldersAll},
				},
			},
			Grants: []string{string(org.RoleAdmin)},
		}

		roles = append(roles, allAnnotationsReaderRole, allAnnotationsWriterRole)
	}

	return hs.accesscontrolService.DeclareFixedRoles(roles...)
}

// Metadata helpers
// getAccessControlMetadata returns the accesscontrol metadata associated with a given resource
func (hs *HTTPServer) getAccessControlMetadata(c *contextmodel.ReqContext,
	orgID int64, prefix string, resourceID string) ac.Metadata {
	ids := map[string]bool{resourceID: true}
	return hs.getMultiAccessControlMetadata(c, prefix, ids)[resourceID]
}

// getMultiAccessControlMetadata returns the accesscontrol metadata associated with a given set of resources
// Context must contain permissions in the given org (see LoadPermissionsMiddleware or AuthorizeInOrgMiddleware)
func (hs *HTTPServer) getMultiAccessControlMetadata(c *contextmodel.ReqContext,
	prefix string, resourceIDs map[string]bool) map[string]ac.Metadata {
	if !c.QueryBool("accesscontrol") {
		return map[string]ac.Metadata{}
	}

	if len(c.SignedInUser.GetPermissions()) == 0 {
		return map[string]ac.Metadata{}
	}

	return ac.GetResourcesMetadata(c.Req.Context(), c.SignedInUser.GetPermissions(), prefix, resourceIDs)
}
