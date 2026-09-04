package applicationapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type applicationEndpointRequest struct {
	DomainID       uint   `json:"domain_id"`
	Path           string `json:"path"`
	ServicePort    int32  `json:"service_port"`
	Protocol       string `json:"protocol"`
	TLSEnabled     bool   `json:"tls_enabled"`
	IngressEnabled *bool  `json:"ingress_enabled"`
	AccessMode     string `json:"access_mode"`
}

func (h *ApplicationHandler) ListApplicationEndpoints(c *gin.Context) {
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	if _, err := h.queries.GetApplication(applicationID); err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	endpoints, err := h.applications.ListApplicationEndpoints(applicationID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	for index := range endpoints {
		endpoints[index].IngressEnabled = endpointUsesIngress(endpoints[index])
		if endpoints[index].Protocol == "" {
			endpoints[index].Protocol = applicationservice.ServiceProtocolTCP
		}
	}
	model.Success(c, endpoints)
}

func (h *ApplicationHandler) CreateApplicationEndpoint(c *gin.Context) {
	if h.kubernetes == nil || !h.kubernetes.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
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
	var req applicationEndpointRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.DomainID == 0 {
		apiShared.BadRequest(c, "请选择受管域名")
		return
	}
	endpoint, err := h.prepareApplicationEndpoint(c.Request.Context(), app, 0, req)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	serviceSpec, err := h.applicationServiceSpec(app)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	servicePort, err := applicationservice.ResolveEndpointServicePort(serviceSpec, req.ServicePort, req.Protocol, endpointUsesIngress(*endpoint))
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	endpoint.ServicePort = servicePort.Port
	endpoint.Protocol = servicePort.Protocol
	endpoints, err := h.applications.ListApplicationEndpoints(app.ID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	endpoints = append(endpoints, *endpoint)
	if err := h.kubernetes.SyncApplicationEndpoints(c.Request.Context(), applicationContextFor(app), endpoints, servicePort.Port); err != nil {
		apiShared.ValidationError(c, fmt.Sprintf("同步应用入口: %v", err))
		return
	}
	if err := h.applications.CreateApplicationEndpoint(endpoint); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	endpoint.IngressEnabled = endpointUsesIngress(*endpoint)
	model.Success(c, endpoint)
}

func (h *ApplicationHandler) UpdateApplicationEndpoint(c *gin.Context) {
	if h.kubernetes == nil || !h.kubernetes.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	endpointID, err := apiShared.ParseID(c.Param("endpointID"))
	if err != nil {
		apiShared.BadRequest(c, "入口 ID 无效")
		return
	}
	app, err := h.queries.GetApplication(applicationID)
	if err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	var req applicationEndpointRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.DomainID == 0 {
		apiShared.BadRequest(c, "请选择受管域名")
		return
	}
	endpoint, err := h.applications.GetApplicationEndpoint(applicationID, endpointID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "应用入口不存在")
		return
	}
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	updated, err := h.prepareApplicationEndpoint(c.Request.Context(), app, endpoint.ID, req)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	serviceSpec, err := h.applicationServiceSpec(app)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	servicePort, err := applicationservice.ResolveEndpointServicePort(serviceSpec, req.ServicePort, req.Protocol, endpointUsesIngress(*updated))
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	updated.ID = endpoint.ID
	updated.CreatedAt = endpoint.CreatedAt
	updated.ServicePort = servicePort.Port
	updated.Protocol = servicePort.Protocol
	endpoints, err := h.applications.ListApplicationEndpoints(app.ID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	for index := range endpoints {
		if endpoints[index].ID == endpoint.ID {
			endpoints[index] = *updated
		}
	}
	if err := h.kubernetes.SyncApplicationEndpoints(c.Request.Context(), applicationContextFor(app), endpoints, servicePort.Port); err != nil {
		apiShared.ValidationError(c, fmt.Sprintf("同步应用入口: %v", err))
		return
	}
	if err := h.applications.UpdateApplicationEndpoint(updated); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	updated.IngressEnabled = endpointUsesIngress(*updated)
	model.Success(c, updated)
}

func (h *ApplicationHandler) DeleteApplicationEndpoint(c *gin.Context) {
	if h.kubernetes == nil || !h.kubernetes.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
	applicationID, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	endpointID, err := apiShared.ParseID(c.Param("endpointID"))
	if err != nil {
		apiShared.BadRequest(c, "入口 ID 无效")
		return
	}
	app, err := h.queries.GetApplication(applicationID)
	if err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	if _, err := h.applications.GetApplicationEndpoint(applicationID, endpointID); errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "应用入口不存在")
		return
	} else if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	serviceSpec, err := h.applicationServiceSpec(app)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	servicePort, hasTCPPort := serviceSpec.PrimaryTCPPort()
	if !hasTCPPort {
		servicePorts := serviceSpec.PortSpecs()
		if len(servicePorts) == 0 {
			apiShared.ValidationError(c, "应用没有可绑定的 Service 端口")
			return
		}
		servicePort = servicePorts[0]
	}
	endpoints, err := h.applications.ListApplicationEndpoints(app.ID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	remaining := make([]model.ApplicationEndpoint, 0, len(endpoints)-1)
	for _, endpoint := range endpoints {
		if endpoint.ID != endpointID {
			remaining = append(remaining, endpoint)
		}
	}
	// Metadata-only UDP bindings remain discoverable and do not require an Ingress.
	if err := h.kubernetes.SyncApplicationEndpoints(c.Request.Context(), applicationContextFor(app), remaining, servicePort.Port); err != nil {
		apiShared.ValidationError(c, fmt.Sprintf("移除应用入口: %v", err))
		return
	}
	if err := h.applications.DeleteApplicationEndpoint(applicationID, endpointID); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	model.Success(c, gin.H{"id": endpointID})
}

func (h *ApplicationHandler) applicationServiceSpec(app *model.Application) (applicationservice.ServiceSpec, error) {
	if release, err := h.applications.GetLatestSuccessfulRelease(app.ID); err == nil {
		var spec applicationservice.ReleaseSpec
		if err := json.Unmarshal([]byte(release.DesiredSpec), &spec); err != nil {
			return applicationservice.ServiceSpec{}, err
		}
		return spec.Service, nil
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return applicationservice.ServiceSpec{}, err
	}
	if template, err := h.applications.GetDefaultApplicationDeploymentTemplate(app.ID); err == nil {
		var spec applicationservice.ReleaseSpec
		if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
			return applicationservice.ServiceSpec{}, err
		}
		return spec.Service, nil
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return applicationservice.ServiceSpec{}, err
	}
	if endpoints, err := h.applications.ListApplicationEndpoints(app.ID); err == nil && len(endpoints) > 0 && endpoints[0].ServicePort > 0 {
		return applicationservice.ServiceSpec{Port: endpoints[0].ServicePort}, nil
	} else if err != nil {
		return applicationservice.ServiceSpec{}, err
	}
	return applicationservice.ServiceSpec{Port: 80}, nil
}

func endpointUsesIngress(endpoint model.ApplicationEndpoint) bool {
	// Empty mode is the legacy representation and must continue to create an
	// Ingress for endpoints written before metadata-only bindings existed.
	return strings.ToLower(strings.TrimSpace(endpoint.IngressMode)) != "metadata"
}

func (h *ApplicationHandler) prepareApplicationEndpoint(ctx context.Context, app *model.Application, endpointID uint, req applicationEndpointRequest) (*model.ApplicationEndpoint, error) {
	domain, err := h.resources.GetManagedDomain(req.DomainID)
	if err != nil || !domain.Enabled {
		return nil, fmt.Errorf("域名不存在或已停用")
	}
	if domain.EnvironmentID == 0 || domain.EnvironmentID != app.EnvironmentID {
		return nil, fmt.Errorf("域名仅可用于当前应用环境")
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		path = "/"
	}
	ingressEnabled := true
	if req.IngressEnabled != nil {
		ingressEnabled = *req.IngressEnabled
	}
	if ingressEnabled {
		conflicts, err := h.applications.CountApplicationEndpointRoute(domain.ID, path, endpointID)
		if err != nil {
			return nil, fmt.Errorf("检查域名路由冲突: %w", err)
		}
		if conflicts > 0 {
			return nil, fmt.Errorf("域名 %q 的路径 %q 已被其他应用入口使用", domain.Hostname, path)
		}
	}
	accessMode := strings.TrimSpace(req.AccessMode)
	if accessMode == "" {
		accessMode = model.ApplicationEndpointAccessPublic
	}
	if accessMode != model.ApplicationEndpointAccessPublic && accessMode != model.ApplicationEndpointAccessProtectedConsole {
		return nil, fmt.Errorf("入口用途无效")
	}
	ingressMode := "ingress"
	if !ingressEnabled {
		ingressMode = "metadata"
	}
	endpoint := &model.ApplicationEndpoint{ApplicationID: app.ID, DomainID: domain.ID, Exposure: applicationservice.ExposurePublic, Domain: domain.Hostname, Path: path, TLSEnabled: req.TLSEnabled, IngressEnabled: ingressEnabled, IngressMode: ingressMode, IssuerRef: domain.IssuerRef, AccessMode: accessMode}
	if !endpoint.TLSEnabled {
		return endpoint, nil
	}
	if domain.CertificateName == "" || domain.TLSSecretName == "" {
		return nil, fmt.Errorf("域名尚未申请证书")
	}
	certificate, err := h.kubernetes.GetCertificateContext(ctx, domain.Namespace, domain.CertificateName)
	if err != nil {
		return nil, fmt.Errorf("读取域名证书: %w", err)
	}
	if certificate.Status != "Ready" {
		detail := certificate.Reason
		if detail == "" {
			detail = "等待 cert-manager 签发"
		}
		return nil, fmt.Errorf("域名证书尚未就绪: %s", detail)
	}
	endpoint.TLSSecretName = domain.TLSSecretName
	return endpoint, nil
}
