package applicationapi

import (
	"context"
	"sort"
	"strings"

	infrastructureapi "github.com/cylism/cylism-manager/internal/api/infrastructure"
	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/application"
	"github.com/cylism/cylism-manager/internal/model"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/gin-gonic/gin"
)

type workspaceRuntimeSummary = applicationservice.WorkspaceRuntimeSummary

type workspaceApplicationInfo struct {
	model.Application
	ActiveRelease      *model.Release          `json:"active_release,omitempty"`
	LatestRelease      *model.Release          `json:"latest_release,omitempty"`
	Runtime            workspaceRuntimeSummary `json:"runtime"`
	EndpointURL        string                  `json:"endpoint_url,omitempty"`
	EndpointAccessMode string                  `json:"endpoint_access_mode,omitempty"`
	EndpointCount      int                     `json:"endpoint_count"`
}

type applicationDiscoveryInfo struct {
	ID            uint                        `json:"id"`
	ProjectID     uint                        `json:"project_id"`
	EnvironmentID uint                        `json:"environment_id"`
	Name          string                      `json:"name"`
	WorkloadKind  string                      `json:"workload_kind"`
	Capabilities  []string                    `json:"capabilities"`
	Environment   applicationEnvironmentInfo  `json:"environment"`
	Endpoints     []applicationPublicEndpoint `json:"endpoints"`
	Runtime       applicationRuntimeInfo      `json:"runtime"`
}

type applicationEnvironmentInfo struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type applicationPublicEndpoint struct {
	Domain         string `json:"domain"`
	Path           string `json:"path"`
	ServicePort    int32  `json:"service_port"`
	Protocol       string `json:"protocol"`
	TLSEnabled     bool   `json:"tls_enabled"`
	IngressEnabled bool   `json:"ingress_enabled"`
}

type applicationRuntimeInfo = applicationservice.RuntimeInfo
type applicationReleaseSummary = applicationservice.ReleaseSummary

func (h *ApplicationHandler) DiscoverApplications(c *gin.Context) {
	projectID, err := apiShared.OptionalID(strings.TrimSpace(c.Query("project_id")))
	if err != nil || projectID == 0 {
		apiShared.BadRequest(c, "项目 ID 必填且必须有效")
		return
	}
	if _, err := h.queries.GetProject(projectID); err != nil {
		apiShared.NotFound(c, "项目不存在")
		return
	}
	environmentID, err := apiShared.OptionalID(strings.TrimSpace(c.Query("environment_id")))
	if err != nil {
		apiShared.BadRequest(c, "环境 ID 无效")
		return
	}
	if environmentID != 0 {
		if _, err := h.queries.ResolveEnvironment(projectID, environmentID); err != nil {
			apiShared.ValidationError(c, "环境不属于所选项目")
			return
		}
	}
	capability := strings.TrimSpace(c.Query("capability"))
	if capability != "" {
		normalized, err := model.NormalizeApplicationCapabilities([]string{capability})
		if err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
		capability = normalized[0]
	}
	applications, err := h.queries.ListApplications(projectID, environmentID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	if capability != "" {
		filtered := applications[:0]
		for _, app := range applications {
			if applicationHasCapability(app, capability) {
				filtered = append(filtered, app)
			}
		}
		applications = filtered
	}
	releases, err := h.queries.ListReleasesByApplications(applicationIDs(applications))
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	runtimes := h.queries.ApplicationRuntimeInfos(c.Request.Context(), K8s, applications, releases)
	infos := make([]applicationDiscoveryInfo, 0, len(applications))
	for _, app := range applications {
		infos = append(infos, applicationDiscoveryInfoFromModel(app, runtimes[app.ID]))
	}
	model.Success(c, infos)
}

func (h *ApplicationHandler) GetApplicationRuntime(c *gin.Context) {
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	app, err := h.queries.GetApplication(applicationID)
	if err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	releases, err := h.queries.ListReleases(applicationID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	runtime := h.queries.ApplicationRuntimeInfos(c.Request.Context(), K8s, []model.Application{*app}, map[uint][]model.Release{applicationID: releases})[applicationID]
	model.Success(c, runtime)
}

func applicationIDs(applications []model.Application) []uint {
	ids := make([]uint, 0, len(applications))
	for _, app := range applications {
		ids = append(ids, app.ID)
	}
	return ids
}

func applicationHasCapability(app model.Application, capability string) bool {
	for _, item := range app.Capabilities {
		if item == capability {
			return true
		}
	}
	return false
}

func applicationDiscoveryInfoFromModel(app model.Application, runtime applicationRuntimeInfo) applicationDiscoveryInfo {
	endpoints := make([]applicationPublicEndpoint, 0, len(app.Endpoints))
	for _, endpoint := range app.Endpoints {
		protocol := endpoint.Protocol
		if protocol == "" {
			protocol = application.ServiceProtocolTCP
		}
		endpoints = append(endpoints, applicationPublicEndpoint{Domain: endpoint.Domain, Path: endpoint.Path, ServicePort: endpoint.ServicePort, Protocol: protocol, TLSEnabled: endpoint.TLSEnabled, IngressEnabled: endpointUsesIngress(endpoint)})
	}
	capabilities := append([]string{}, app.Capabilities...)
	return applicationDiscoveryInfo{
		ID: app.ID, ProjectID: app.ProjectID, EnvironmentID: app.EnvironmentID, Name: app.Name, WorkloadKind: app.WorkloadKind, Capabilities: capabilities,
		Environment: applicationEnvironmentInfo{ID: app.Environment.ID, Name: app.Environment.Name, Namespace: app.Environment.Namespace}, Endpoints: endpoints, Runtime: runtime,
	}
}

func (h *ApplicationHandler) WorkspaceOverview(c *gin.Context) {
	projectID, err := apiShared.OptionalID(strings.TrimSpace(c.Query("project_id")))
	if err != nil || projectID == 0 {
		apiShared.BadRequest(c, "项目 ID 必填且必须有效")
		return
	}
	environmentID, err := apiShared.OptionalID(strings.TrimSpace(c.Query("environment_id")))
	if err != nil || environmentID == 0 {
		apiShared.BadRequest(c, "环境 ID 必填且必须有效")
		return
	}
	project, err := h.queries.GetProject(projectID)
	if err != nil {
		apiShared.NotFound(c, "项目不存在")
		return
	}
	environment, err := h.queries.ResolveEnvironment(projectID, environmentID)
	if err != nil {
		apiShared.ValidationError(c, "环境不属于所选项目")
		return
	}
	applications, err := h.queries.ListApplications(projectID, environmentID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	domains, err := h.resources.ListManagedDomains(environmentID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	type workspaceRelease struct {
		model.Release
		ApplicationName string `json:"application_name"`
	}
	failedReleases := make([]workspaceRelease, 0)
	recentReleases := make([]workspaceRelease, 0)
	applicationIDs := make([]uint, 0, len(applications))
	for _, app := range applications {
		applicationIDs = append(applicationIDs, app.ID)
	}
	allReleases, releaseErr := h.queries.ListReleasesByApplications(applicationIDs)
	if releaseErr == nil {
		for _, app := range applications {
			releases := allReleases[app.ID]
			for _, release := range releases {
				item := workspaceRelease{Release: release, ApplicationName: app.Name}
				recentReleases = append(recentReleases, item)
				if release.Status == model.ReleaseStatusFailed {
					failedReleases = append(failedReleases, item)
				}
			}
		}
	}
	sort.Slice(recentReleases, func(i, j int) bool { return recentReleases[i].CreatedAt.After(recentReleases[j].CreatedAt) })
	if len(recentReleases) > 8 {
		recentReleases = recentReleases[:8]
	}
	workspaceApplications := h.workspaceApplicationInfos(c.Request.Context(), environment.Namespace, applications, allReleases)
	domainInfos := make([]infrastructureapi.ManagedDomainInfo, 0, len(domains))
	for index := range domains {
		domainInfos = append(domainInfos, infrastructureapi.ManagedDomainInfoFor(&domains[index], K8s, h.resources))
	}
	model.Success(c, gin.H{"project": project, "environment": environment, "applications": workspaceApplications, "domains": domainInfos, "failed_releases": failedReleases, "recent_releases": recentReleases})
}

func (h *ApplicationHandler) workspaceApplicationInfos(ctx context.Context, namespace string, applications []model.Application, allReleases map[uint][]model.Release) []workspaceApplicationInfo {
	podStates, runtimeAvailable := h.queries.WorkspacePodStates(ctx, K8s, namespace)
	infos := make([]workspaceApplicationInfo, 0, len(applications))
	for _, app := range applications {
		var latest, active *model.Release
		for index := range allReleases[app.ID] {
			release := &allReleases[app.ID][index]
			if latest == nil {
				latest = release
			}
			if active == nil && release.Status == model.ReleaseStatusSucceeded {
				active = release
			}
		}
		runtime := workspaceRuntimeSummary{Status: "not_released"}
		if latest != nil && releaseInProgress(latest.Status) {
			runtime.Status = "deploying"
		} else if active != nil {
			if !runtimeAvailable {
				runtime.Status = "unknown"
			} else {
				runtime = podStates[applicationservice.WorkspacePodKey(app.Name, active.Sequence)]
				if runtime.Status == "" {
					runtime.Status = "unavailable"
				}
			}
		} else if latest != nil {
			runtime.Status = "unavailable"
		}
		infos = append(infos, workspaceApplicationInfo{Application: app, ActiveRelease: active, LatestRelease: latest, Runtime: runtime, EndpointURL: applicationEndpointURL(app), EndpointAccessMode: applicationEndpointAccessMode(app), EndpointCount: len(app.Endpoints)})
	}
	return infos
}

func releaseInProgress(status string) bool {
	switch status {
	case model.ReleaseStatusDraft, model.ReleaseStatusValidating, model.ReleaseStatusApplying, model.ReleaseStatusWaitingReady, model.ReleaseStatusVerifying, model.ReleaseStatusRollingBack:
		return true
	default:
		return false
	}
}

func applicationEndpointURL(app model.Application) string {
	if len(app.Endpoints) == 0 || strings.TrimSpace(app.Endpoints[0].Domain) == "" {
		return ""
	}
	endpoint := app.Endpoints[0]
	if endpoint.AccessMode == model.ApplicationEndpointAccessProtectedConsole {
		return ""
	}
	scheme := "http"
	if endpoint.TLSEnabled {
		scheme = "https"
	}
	return scheme + "://" + endpoint.Domain + strings.TrimSpace(endpoint.Path)
}

func applicationEndpointAccessMode(app model.Application) string {
	if len(app.Endpoints) == 0 {
		return ""
	}
	mode := app.Endpoints[0].AccessMode
	if mode == "" {
		return model.ApplicationEndpointAccessPublic
	}
	return mode
}
