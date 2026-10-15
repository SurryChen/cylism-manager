package delivery

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	registryservice "github.com/cylism/cylism-manager/internal/service/registry"
	"github.com/gin-gonic/gin"
)

type registryProxyRelayStore interface {
	repository.RegistryProxyRepository
	repository.RegistryProxyRelayRepository
	GetServer(uint) (*model.Server, error)
	ListServers() ([]model.Server, error)
}

type RegistryProxyRelayHandler struct {
	store   registryProxyRelayStore
	service *registryservice.ProxyRelayService
}

func NewRegistryProxyRelayHandler(store registryProxyRelayStore, service *registryservice.ProxyRelayService) *RegistryProxyRelayHandler {
	return &RegistryProxyRelayHandler{store: store, service: service}
}

func (h *RegistryProxyRelayHandler) proxy(c *gin.Context) (*model.RegistryProxy, bool) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "代理 ID 无效")
		return nil, false
	}
	proxy, err := h.store.GetRegistryProxyByID(id)
	if err != nil {
		apiShared.ValidationError(c, "代理不存在")
		return nil, false
	}
	return proxy, true
}

func (h *RegistryProxyRelayHandler) List(c *gin.Context) {
	proxy, ok := h.proxy(c)
	if !ok {
		return
	}
	relays, err := h.store.ListRegistryProxyRelays(proxy.ID)
	if err != nil {
		apiShared.DBError(c, "读取中转入口失败")
		return
	}
	apiShared.Success(c, relays)
}

func (h *RegistryProxyRelayHandler) Create(c *gin.Context) {
	proxy, ok := h.proxy(c)
	if !ok {
		return
	}
	var req struct {
		ServerID   uint   `json:"server_id"`
		ListenIP   string `json:"listen_ip"`
		ListenPort int    `json:"listen_port"`
	}
	if c.ShouldBindJSON(&req) != nil {
		apiShared.BadRequest(c, "中转配置无效")
		return
	}
	req.ListenIP = strings.TrimSpace(req.ListenIP)
	if !registryservice.PrivateOrSharedAddressIP(req.ListenIP) || net.ParseIP(req.ListenIP).To4() == nil || req.ListenPort < 1024 || req.ListenPort > 65535 {
		apiShared.ValidationError(c, "请填写目标机器的私网或共享 IP，以及 1024-65535 端口")
		return
	}
	if proxy.Status != "ready" {
		apiShared.ValidationError(c, "源代理尚未就绪")
		return
	}
	server, err := h.store.GetServer(req.ServerID)
	if err != nil {
		apiShared.ValidationError(c, "中转机器不存在")
		return
	}
	relays, err := h.store.ListRegistryProxyRelays(proxy.ID)
	if err != nil {
		apiShared.DBError(c, "检查中转端口失败")
		return
	}
	for _, existing := range relays {
		if existing.ServerID == req.ServerID && existing.ListenPort == req.ListenPort {
			apiShared.ValidationError(c, "该机器的中转端口已被本代理使用")
			return
		}
	}
	relay := &model.RegistryProxyRelay{ProxyID: proxy.ID, ServerID: server.ID, ListenIP: req.ListenIP, ListenPort: req.ListenPort, Status: "deploying"}
	if err := h.store.SaveRegistryProxyRelay(relay); err != nil {
		apiShared.DBError(c, "保存中转入口失败")
		return
	}
	h.deploy(c, server, relay, proxy)
}

func (h *RegistryProxyRelayHandler) Retry(c *gin.Context) {
	proxy, ok := h.proxy(c)
	if !ok {
		return
	}
	relay, ok := h.relay(c, proxy.ID)
	if !ok {
		return
	}
	if relay.Status != "failed" {
		apiShared.ValidationError(c, "仅失败的中转入口可以重试")
		return
	}
	server, err := h.store.GetServer(relay.ServerID)
	if err != nil {
		apiShared.ValidationError(c, "中转机器不存在")
		return
	}
	relay.Status, relay.LastError = "deploying", ""
	if err := h.store.SaveRegistryProxyRelay(relay); err != nil {
		apiShared.DBError(c, "保存中转状态失败")
		return
	}
	h.deploy(c, server, relay, proxy)
}

func (h *RegistryProxyRelayHandler) deploy(c *gin.Context, server *model.Server, relay *model.RegistryProxyRelay, proxy *model.RegistryProxy) {
	err := h.service.Deploy(c.Request.Context(), server, relay, proxy)
	now := time.Now().UTC()
	relay.LastCheckedAt = &now
	if err != nil {
		relay.Status = "failed"
		relay.LastError = err.Error()
	} else {
		relay.Status = "ready"
		relay.LastError = ""
	}
	if saveErr := h.store.SaveRegistryProxyRelay(relay); saveErr != nil {
		apiShared.DBError(c, "保存中转状态失败")
		return
	}
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, relay)
}

func (h *RegistryProxyRelayHandler) relay(c *gin.Context, proxyID uint) (*model.RegistryProxyRelay, bool) {
	id, err := apiShared.ParseID(c.Param("relayID"))
	if err != nil {
		apiShared.BadRequest(c, "中转 ID 无效")
		return nil, false
	}
	relay, err := h.store.GetRegistryProxyRelay(id)
	if err != nil || relay.ProxyID != proxyID {
		apiShared.ValidationError(c, "中转入口不存在")
		return nil, false
	}
	return relay, true
}

func (h *RegistryProxyRelayHandler) Check(c *gin.Context) {
	proxy, ok := h.proxy(c)
	if !ok {
		return
	}
	relay, ok := h.relay(c, proxy.ID)
	if !ok {
		return
	}
	server, err := h.store.GetServer(relay.ServerID)
	if err != nil {
		apiShared.ValidationError(c, "中转机器不存在")
		return
	}
	err = h.service.Check(c.Request.Context(), server, relay)
	now := time.Now().UTC()
	relay.LastCheckedAt = &now
	if err != nil {
		relay.Status = "unhealthy"
		relay.LastError = err.Error()
	} else {
		relay.Status = "ready"
		relay.LastError = ""
	}
	if saveErr := h.store.SaveRegistryProxyRelay(relay); saveErr != nil {
		apiShared.DBError(c, "保存检查结果失败")
		return
	}
	apiShared.Success(c, relay)
}

func (h *RegistryProxyRelayHandler) Delete(c *gin.Context) {
	proxy, ok := h.proxy(c)
	if !ok {
		return
	}
	relay, ok := h.relay(c, proxy.ID)
	if !ok {
		return
	}
	endpoint := fmt.Sprintf("http://%s:%d", relay.ListenIP, relay.ListenPort)
	servers, err := h.store.ListServers()
	if err != nil {
		apiShared.DBError(c, "读取集群节点失败")
		return
	}
	for i := range servers {
		server := &servers[i]
		if server.ClusterRole == "" || (proxy.ClusterID != 0 && (server.ClusterID == nil || *server.ClusterID != proxy.ClusterID)) {
			continue
		}
		used, checkErr := h.service.NodeUsesEndpoint(c.Request.Context(), server, registryservice.NormalizeRegistryHost(proxy.Registry), endpoint)
		if checkErr != nil {
			apiShared.ValidationError(c, fmt.Sprintf("无法确认节点 %s 的镜像源配置: %v", server.Name, checkErr))
			return
		}
		if used {
			apiShared.ValidationError(c, fmt.Sprintf("节点 %s 仍在使用此中转入口，请先修改节点镜像源", server.Name))
			return
		}
	}
	routes, err := h.store.ListRegistryProxyNodeRoutes(proxy.ID)
	if err != nil {
		apiShared.DBError(c, "检查节点引用失败")
		return
	}
	server, err := h.store.GetServer(relay.ServerID)
	if err != nil {
		apiShared.ValidationError(c, "中转机器不存在")
		return
	}
	if err := h.service.Remove(c.Request.Context(), server, relay); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	for _, route := range routes {
		if route.Endpoint == endpoint {
			if err := h.store.DeleteRegistryProxyNodeRoute(proxy.ID, route.ServerID); err != nil {
				apiShared.DBError(c, "移除旧节点入口记录失败")
				return
			}
		}
	}
	if err := h.store.DeleteRegistryProxyRelay(relay.ID); err != nil {
		apiShared.DBError(c, "删除中转记录失败")
		return
	}
	apiShared.Success(c, gin.H{"deleted": true})
}

func (h *RegistryProxyRelayHandler) ListRoutes(c *gin.Context) {
	proxy, ok := h.proxy(c)
	if !ok {
		return
	}
	routes, err := h.store.ListRegistryProxyNodeRoutes(proxy.ID)
	if err != nil {
		apiShared.DBError(c, "读取节点入口失败")
		return
	}
	apiShared.Success(c, routes)
}

func (h *RegistryProxyRelayHandler) node(c *gin.Context, proxy *model.RegistryProxy) (*model.Server, bool) {
	id, err := apiShared.ParseID(c.Param("serverID"))
	if err != nil {
		apiShared.BadRequest(c, "节点 ID 无效")
		return nil, false
	}
	server, err := h.store.GetServer(id)
	if err != nil || server.ClusterRole == "" || (proxy.ClusterID != 0 && (server.ClusterID == nil || *server.ClusterID != proxy.ClusterID)) {
		apiShared.ValidationError(c, "节点不属于代理所在集群")
		return nil, false
	}
	return server, true
}

func (h *RegistryProxyRelayHandler) SwitchRoute(c *gin.Context) {
	proxy, ok := h.proxy(c)
	if !ok {
		return
	}
	server, ok := h.node(c, proxy)
	if !ok {
		return
	}
	var req struct {
		RelayID uint `json:"relay_id"`
	}
	if c.ShouldBindJSON(&req) != nil {
		apiShared.BadRequest(c, "入口选择无效")
		return
	}
	endpoint := "http://" + proxy.EndpointHost + ":" + strconv.Itoa(int(proxy.NodePort))
	if req.RelayID != 0 {
		relay, err := h.store.GetRegistryProxyRelay(req.RelayID)
		if err != nil || relay.ProxyID != proxy.ID {
			apiShared.ValidationError(c, "中转入口不存在")
			return
		}
		relayServer, err := h.store.GetServer(relay.ServerID)
		if err != nil {
			apiShared.ValidationError(c, "中转机器不存在")
			return
		}
		if err := h.service.Check(c.Request.Context(), relayServer, relay); err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
		endpoint = fmt.Sprintf("http://%s:%d", relay.ListenIP, relay.ListenPort)
	}
	if err := h.service.SwitchNode(c.Request.Context(), server, registryservice.NormalizeRegistryHost(proxy.Registry), endpoint); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	verifyErr := h.service.CheckNodeRoute(c.Request.Context(), server, registryservice.NormalizeRegistryHost(proxy.Registry), endpoint)
	now := time.Now().UTC()
	route := &model.RegistryProxyNodeRoute{ProxyID: proxy.ID, ServerID: server.ID, Endpoint: endpoint, Status: "ready", LastCheckedAt: &now}
	if verifyErr != nil {
		route.Status = "failed"
		route.LastError = verifyErr.Error()
	}
	if err := h.store.SaveRegistryProxyNodeRoute(route); err != nil {
		apiShared.DBError(c, "保存节点入口失败")
		return
	}
	if verifyErr != nil {
		apiShared.ValidationError(c, "入口已切换，但验证失败: "+verifyErr.Error())
		return
	}
	apiShared.Success(c, route)
}

func (h *RegistryProxyRelayHandler) CheckRoute(c *gin.Context) {
	proxy, ok := h.proxy(c)
	if !ok {
		return
	}
	server, ok := h.node(c, proxy)
	if !ok {
		return
	}
	routes, err := h.store.ListRegistryProxyNodeRoutes(proxy.ID)
	if err != nil {
		apiShared.DBError(c, "读取节点入口失败")
		return
	}
	for i := range routes {
		route := &routes[i]
		if route.ServerID != server.ID {
			continue
		}
		err = h.service.CheckNodeRoute(c.Request.Context(), server, registryservice.NormalizeRegistryHost(proxy.Registry), route.Endpoint)
		now := time.Now().UTC()
		route.LastCheckedAt = &now
		if err != nil {
			route.Status = "failed"
			route.LastError = err.Error()
		} else {
			route.Status = "ready"
			route.LastError = ""
		}
		if saveErr := h.store.SaveRegistryProxyNodeRoute(route); saveErr != nil {
			apiShared.DBError(c, "保存检查结果失败")
			return
		}
		apiShared.Success(c, route)
		return
	}
	apiShared.Error(c, http.StatusNotFound, apiShared.CodeNotFound, "节点尚未切换入口")
}

func (h *RegistryProxyRelayHandler) ForgetRoute(c *gin.Context) {
	proxy, ok := h.proxy(c)
	if !ok {
		return
	}
	server, ok := h.node(c, proxy)
	if !ok {
		return
	}
	routes, err := h.store.ListRegistryProxyNodeRoutes(proxy.ID)
	if err != nil {
		apiShared.DBError(c, "读取节点入口失败")
		return
	}
	for _, route := range routes {
		if route.ServerID != server.ID {
			continue
		}
		if err := h.service.ConfirmNodeRouteChanged(c.Request.Context(), server, registryservice.NormalizeRegistryHost(proxy.Registry), route.Endpoint); err != nil {
			apiShared.ValidationError(c, err.Error())
			return
		}
		if err := h.store.DeleteRegistryProxyNodeRoute(proxy.ID, server.ID); err != nil {
			apiShared.DBError(c, "移除节点入口记录失败")
			return
		}
		apiShared.Success(c, gin.H{"deleted": true})
		return
	}
	apiShared.NotFound(c, "节点入口记录不存在")
}
