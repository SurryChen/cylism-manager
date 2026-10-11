package cluster

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/cylism/cylism-manager/internal/transport"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

type crossServiceStore interface {
	GetCluster(uint) (*model.Cluster, error)
	GetServer(uint) (*model.Server, error)
	ListServers() ([]model.Server, error)
	ListAllCrossClusterServices() ([]model.CrossClusterService, error)
	ListCrossClusterServices(uint) ([]model.CrossClusterService, error)
	GetCrossClusterService(uint, uint) (*model.CrossClusterService, error)
	CreateCrossClusterService(*model.CrossClusterService) error
	UpdateCrossClusterServiceState(uint, string, string) error
	DeleteCrossClusterService(uint) error
	RecordCrossClusterServiceTask(uint, uint, string, string, bool) error
}

type CrossServiceHandler struct {
	store  crossServiceStore
	key    []byte
	client func(context.Context, uint) (*k8s.Client, error)
	ssh    func(context.Context, time.Duration, *model.Server, []byte, string) ([]byte, error)
	status func(context.Context, *model.Server, []byte) tailscaleStatus
}

func NewCrossServiceHandler(st crossServiceStore, key []byte, client func(context.Context, uint) (*k8s.Client, error)) *CrossServiceHandler {
	return &CrossServiceHandler{store: st, key: key, client: client, ssh: transport.SSHExecServerContext, status: inspectTailscale}
}

type crossServiceCheck struct {
	Ready          bool     `json:"ready"`
	Message        string   `json:"message,omitempty"`
	SourceNodes    []string `json:"source_nodes"`
	TargetIP       string   `json:"target_ip,omitempty"`
	TargetServerID uint     `json:"target_server_id,omitempty"`
	source         *model.Cluster
	target         *model.Cluster
	targetServer   *model.Server
	proxyBinary    string
}

func (h *CrossServiceHandler) check(ctx context.Context, sourceID, targetID uint) (*crossServiceCheck, error) {
	result := &crossServiceCheck{SourceNodes: []string{}}
	if sourceID == targetID || targetID == 0 {
		result.Message = "请选择其他目标集群"
		return result, nil
	}
	source, err := h.store.GetCluster(sourceID)
	if err != nil {
		return nil, err
	}
	target, err := h.store.GetCluster(targetID)
	if err != nil {
		return nil, err
	}
	result.source, result.target = source, target
	if source.Status != "ready" || target.Status != "ready" || source.ControlServerID == nil || target.ControlServerID == nil {
		result.Message = "两个集群都需要先部署控制平面并处于运行状态"
		return result, nil
	}
	servers, err := h.store.ListServers()
	if err != nil {
		return nil, err
	}
	var targetServer *model.Server
	for i := range servers {
		server := &servers[i]
		if server.ID == *target.ControlServerID {
			targetServer = server
		}
		if server.ClusterID == nil || *server.ClusterID != sourceID {
			continue
		}
		if server.ClusterRole != "control-plane" && server.ClusterRole != "worker" {
			continue
		}
		status := h.status(ctx, server, h.key)
		if status.Status != "online" || net.ParseIP(status.IP) == nil {
			result.Message = "来源集群的服务器尚未全部接入 Tailscale，请先到组网页检查"
			return result, nil
		}
		result.SourceNodes = append(result.SourceNodes, server.Name)
	}
	if len(result.SourceNodes) == 0 || targetServer == nil {
		result.Message = "集群节点信息不完整，请先检查拓扑"
		return result, nil
	}
	status := h.status(ctx, targetServer, h.key)
	if status.Status != "online" || net.ParseIP(status.IP) == nil {
		result.Message = "目标集群控制平面尚未接入 Tailscale，请先到组网页检查"
		return result, nil
	}
	result.TargetIP, result.TargetServerID, result.targetServer = status.IP, targetServer.ID, targetServer
	for i := range servers {
		server := &servers[i]
		if server.ClusterID == nil || *server.ClusterID != sourceID || (server.ClusterRole != "control-plane" && server.ClusterRole != "worker") {
			continue
		}
		command := "tailscale ping --c=1 --timeout=5s " + shellQuote(status.IP)
		if _, err := h.ssh(ctx, 10*time.Second, server, h.key, command); err != nil {
			result.Message = server.Name + " 无法连接目标集群的 Tailscale 节点，请检查网络或 ACL"
			return result, nil
		}
	}
	if h.client == nil {
		result.Message = "集群 Agent 未配置"
		return result, nil
	}
	for _, id := range []uint{sourceID, targetID} {
		client, err := h.client(ctx, id)
		if err != nil || client == nil || client.Clientset == nil {
			result.Message = "集群 Agent 不可用，请先到 Agent 管理检查两个集群"
			return result, nil
		}
		if _, err := client.Clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1}); err != nil {
			result.Message = "集群 Agent 无法读取 Kubernetes API，请先检查两个集群"
			return result, nil
		}
	}
	output, err := h.ssh(ctx, 10*time.Second, targetServer, h.key, privilegedCommand(targetServer, "command -v systemd-socket-proxyd || for p in /usr/lib/systemd/systemd-socket-proxyd /lib/systemd/systemd-socket-proxyd /usr/libexec/systemd/systemd-socket-proxyd; do if [ -x \"$p\" ]; then printf '%s' \"$p\"; break; fi; done"))
	if err != nil {
		result.Message = "无法在目标服务器检查转发组件：" + err.Error()
		return result, nil
	}
	path := strings.TrimSpace(string(output))
	if !strings.HasSuffix(path, "/systemd-socket-proxyd") || strings.ContainsAny(path, " \r\n") {
		result.Message = "目标服务器缺少 systemd-socket-proxyd"
		return result, nil
	}
	result.proxyBinary = path
	result.Ready = true
	return result, nil
}

func (h *CrossServiceHandler) Precheck(c *gin.Context) {
	sourceID, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "集群 ID 无效")
		return
	}
	targetID, err := apiShared.ParsePositiveID(c.Query("target_cluster_id"))
	if err != nil {
		apiShared.BadRequest(c, "请选择目标集群")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 75*time.Second)
	defer cancel()
	result, err := h.check(ctx, sourceID, targetID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "集群不存在")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "跨集群连接预检失败")
		return
	}
	apiShared.Success(c, result)
}

func (h *CrossServiceHandler) List(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "集群 ID 无效")
		return
	}
	if _, err := h.store.GetCluster(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apiShared.NotFound(c, "集群不存在")
		} else {
			apiShared.InternalError(c, "读取集群失败")
		}
		return
	}
	items, err := h.store.ListCrossClusterServices(id)
	if err != nil {
		apiShared.InternalError(c, "读取跨集群服务失败")
		return
	}
	apiShared.Success(c, items)
}

func (h *CrossServiceHandler) ListAll(c *gin.Context) {
	items, err := h.store.ListAllCrossClusterServices()
	if err != nil {
		apiShared.InternalError(c, "读取跨集群服务失败")
		return
	}
	apiShared.Success(c, items)
}

type createCrossServiceRequest struct {
	TargetClusterID   uint   `json:"target_cluster_id"`
	SourceNamespace   string `json:"source_namespace"`
	SourceName        string `json:"source_name"`
	SourcePort        int32  `json:"source_port"`
	TargetNamespace   string `json:"target_namespace"`
	TargetName        string `json:"target_name"`
	TargetServicePort int32  `json:"target_service_port"`
}

func (h *CrossServiceHandler) Create(c *gin.Context) {
	sourceID, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "集群 ID 无效")
		return
	}
	var request createCrossServiceRequest
	if c.ShouldBindJSON(&request) != nil || request.TargetClusterID == 0 || request.SourcePort < 1 || request.SourcePort > 65535 || request.TargetServicePort < 1 || request.TargetServicePort > 65535 ||
		len(validation.IsDNS1123Label(request.SourceNamespace)) > 0 || len(validation.IsDNS1035Label(request.SourceName)) > 0 || len(request.SourceName) > 57 || len(validation.IsDNS1123Label(request.TargetNamespace)) > 0 || len(validation.IsDNS1035Label(request.TargetName)) > 0 {
		apiShared.BadRequest(c, "请填写有效的集群、命名空间、Service 名称与端口")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	check, err := h.check(ctx, sourceID, request.TargetClusterID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "集群不存在")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "跨集群连接预检失败")
		return
	}
	if !check.Ready {
		apiShared.BadRequest(c, check.Message)
		return
	}
	sourceClient, err := h.client(ctx, sourceID)
	if err != nil {
		apiShared.BadRequest(c, "来源集群 Agent 不可用："+err.Error())
		return
	}
	targetClient, err := h.client(ctx, request.TargetClusterID)
	if err != nil {
		apiShared.BadRequest(c, "目标集群 Agent 不可用："+err.Error())
		return
	}
	if _, err := sourceClient.Clientset.CoreV1().Namespaces().Get(ctx, request.SourceNamespace, metav1.GetOptions{}); err != nil {
		apiShared.BadRequest(c, "来源命名空间不可用："+err.Error())
		return
	}
	if _, err := sourceClient.Clientset.CoreV1().Services(request.SourceNamespace).Get(ctx, request.SourceName, metav1.GetOptions{}); err == nil {
		apiShared.Conflict(c, "来源集群已有同名 Service，请换一个访问名称")
		return
	} else if !apierrors.IsNotFound(err) {
		apiShared.BadRequest(c, "检查来源 Service 失败："+err.Error())
		return
	}
	targetService, err := targetClient.Clientset.CoreV1().Services(request.TargetNamespace).Get(ctx, request.TargetName, metav1.GetOptions{})
	if err != nil {
		apiShared.BadRequest(c, "目标 Service 不可用："+err.Error())
		return
	}
	if targetService.Spec.ClusterIP == "" || targetService.Spec.ClusterIP == corev1.ClusterIPNone || net.ParseIP(targetService.Spec.ClusterIP) == nil {
		apiShared.BadRequest(c, "暂不支持无头 Service 或没有 ClusterIP 的 Service")
		return
	}
	validPort := false
	for _, port := range targetService.Spec.Ports {
		if port.Port == request.TargetServicePort && (port.Protocol == "" || port.Protocol == corev1.ProtocolTCP) {
			validPort = true
			break
		}
	}
	if !validPort {
		apiShared.BadRequest(c, "目标 Service 不存在所选 TCP 端口")
		return
	}
	item := &model.CrossClusterService{SourceClusterID: sourceID, TargetClusterID: request.TargetClusterID, SourceNamespace: request.SourceNamespace, SourceName: request.SourceName, SourcePort: request.SourcePort, TargetNamespace: request.TargetNamespace, TargetName: request.TargetName, TargetServicePort: request.TargetServicePort, TargetServerID: check.TargetServerID, TailscaleIP: check.TargetIP, Status: "pending"}
	if err := h.store.CreateCrossClusterService(item); err != nil {
		if errors.Is(err, store.ErrCrossClusterServiceConflict) {
			apiShared.Conflict(c, "该跨集群服务名称已存在")
			return
		}
		apiShared.InternalError(c, "创建跨集群服务记录失败")
		return
	}
	err = h.installGateway(ctx, check.targetServer, item, targetService.Spec.ClusterIP, check.proxyBinary)
	if err == nil {
		err = h.probeGateway(ctx, item)
		if err == nil {
			err = h.createSourceResources(ctx, sourceClient, item)
		}
	}
	if err != nil {
		message := "创建跨集群服务失败：" + err.Error()
		_ = h.store.UpdateCrossClusterServiceState(item.ID, "failed", message)
		_ = h.store.RecordCrossClusterServiceTask(sourceID, *check.source.ControlServerID, "cross-create", message, false)
		apiShared.BadRequest(c, message)
		return
	}
	if err := h.store.UpdateCrossClusterServiceState(item.ID, "active", ""); err != nil {
		apiShared.InternalError(c, "服务已创建，但保存状态失败")
		return
	}
	item.Status = "active"
	_ = h.store.RecordCrossClusterServiceTask(sourceID, *check.source.ControlServerID, "cross-create", "已发布 "+request.TargetNamespace+"/"+request.TargetName+" 到 "+request.SourceNamespace+"/"+request.SourceName, true)
	apiShared.Success(c, item)
}

func crossServiceUnit(id uint) string {
	return "cylism-cross-service-" + strconv.FormatUint(uint64(id), 10)
}

func (h *CrossServiceHandler) installGateway(ctx context.Context, server *model.Server, item *model.CrossClusterService, clusterIP, binary string) error {
	unit := crossServiceUnit(item.ID)
	socket := "[Unit]\nDescription=Cylism cross-cluster service " + strconv.FormatUint(uint64(item.ID), 10) + "\n[Socket]\nListenStream=" + net.JoinHostPort(item.TailscaleIP, strconv.Itoa(int(item.GatewayPort))) + "\n[Install]\nWantedBy=sockets.target\n"
	service := "[Unit]\nDescription=Cylism cross-cluster service proxy\n[Service]\nExecStart=" + binary + " " + net.JoinHostPort(clusterIP, strconv.Itoa(int(item.TargetServicePort))) + "\nDynamicUser=yes\nNoNewPrivileges=yes\n"
	base := "/etc/systemd/system/" + unit
	command := "set -e; printf %s " + shellQuote(socket) + " > " + base + ".socket.tmp; printf %s " + shellQuote(service) + " > " + base + ".service.tmp; chmod 0644 " + base + ".socket.tmp " + base + ".service.tmp; mv " + base + ".socket.tmp " + base + ".socket; mv " + base + ".service.tmp " + base + ".service; systemctl daemon-reload; systemctl enable --now " + unit + ".socket; systemctl is-active --quiet " + unit + ".socket"
	output, err := h.ssh(ctx, 25*time.Second, server, h.key, privilegedCommand(server, command))
	if err != nil {
		return fmt.Errorf("目标节点启动私网入口失败: %s (%w)", trimCommandOutput(output), err)
	}
	return nil
}

func (h *CrossServiceHandler) removeGateway(ctx context.Context, server *model.Server, id uint) error {
	unit := crossServiceUnit(id)
	base := "/etc/systemd/system/" + unit
	command := "systemctl disable --now " + unit + ".socket 2>/dev/null || true; systemctl stop " + unit + ".service 2>/dev/null || true; rm -f " + base + ".socket " + base + ".service " + base + ".socket.tmp " + base + ".service.tmp; systemctl daemon-reload"
	output, err := h.ssh(ctx, 20*time.Second, server, h.key, privilegedCommand(server, command))
	if err != nil {
		return fmt.Errorf("清理目标入口失败: %s (%w)", trimCommandOutput(output), err)
	}
	return nil
}

func crossServiceOwner(id uint) string { return strconv.FormatUint(uint64(id), 10) }

func (h *CrossServiceHandler) probeGateway(ctx context.Context, item *model.CrossClusterService) error {
	servers, err := h.store.ListServers()
	if err != nil {
		return err
	}
	checked := 0
	for i := range servers {
		server := &servers[i]
		if server.ClusterID == nil || *server.ClusterID != item.SourceClusterID || (server.ClusterRole != "control-plane" && server.ClusterRole != "worker") {
			continue
		}
		command := "timeout 5 bash -c " + shellQuote("exec 3<>/dev/tcp/"+item.TailscaleIP+"/"+strconv.Itoa(int(item.GatewayPort)))
		if _, err := h.ssh(ctx, 10*time.Second, server, h.key, command); err != nil {
			return fmt.Errorf("%s 无法连接目标私网入口，请检查 Tailscale ACL 与防火墙: %w", server.Name, err)
		}
		checked++
	}
	if checked == 0 {
		return fmt.Errorf("没有可检测的来源节点")
	}
	return nil
}

func (h *CrossServiceHandler) createSourceResources(ctx context.Context, client *k8s.Client, item *model.CrossClusterService) error {
	owner := map[string]string{"cylism.io/cross-service-id": crossServiceOwner(item.ID)}
	protocol := corev1.ProtocolTCP
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: item.SourceName, Namespace: item.SourceNamespace, Annotations: owner}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "tcp", Protocol: protocol, Port: item.SourcePort}}}}
	if _, err := client.Clientset.CoreV1().Services(item.SourceNamespace).Create(ctx, service, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("创建来源 Service 失败: %w", err)
	}
	ready := true
	name := "tcp"
	endpoint := &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: item.SourceName + "-cross", Namespace: item.SourceNamespace, Labels: map[string]string{discoveryv1.LabelServiceName: item.SourceName, discoveryv1.LabelManagedBy: "cylism-manager"}, Annotations: owner}, AddressType: discoveryv1.AddressTypeIPv4, Ports: []discoveryv1.EndpointPort{{Name: &name, Protocol: &protocol, Port: &item.GatewayPort}}, Endpoints: []discoveryv1.Endpoint{{Addresses: []string{item.TailscaleIP}, Conditions: discoveryv1.EndpointConditions{Ready: &ready}}}}
	if _, err := client.Clientset.DiscoveryV1().EndpointSlices(item.SourceNamespace).Create(ctx, endpoint, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("创建来源 EndpointSlice 失败: %w", err)
	}
	return nil
}

func (h *CrossServiceHandler) removeSourceResources(ctx context.Context, client *k8s.Client, item *model.CrossClusterService) error {
	name := item.SourceName + "-cross"
	slice, err := client.Clientset.DiscoveryV1().EndpointSlices(item.SourceNamespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		if slice.Annotations["cylism.io/cross-service-id"] != crossServiceOwner(item.ID) {
			return fmt.Errorf("EndpointSlice 所有权已变化，停止清理")
		}
		if err := client.Clientset.DiscoveryV1().EndpointSlices(item.SourceNamespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	service, err := client.Clientset.CoreV1().Services(item.SourceNamespace).Get(ctx, item.SourceName, metav1.GetOptions{})
	if err == nil {
		if service.Annotations["cylism.io/cross-service-id"] != crossServiceOwner(item.ID) {
			return fmt.Errorf("Service 所有权已变化，停止清理")
		}
		if err := client.Clientset.CoreV1().Services(item.SourceNamespace).Delete(ctx, item.SourceName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (h *CrossServiceHandler) Delete(c *gin.Context) {
	clusterID, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "集群 ID 无效")
		return
	}
	id, err := apiShared.ParsePositiveID(c.Param("serviceID"))
	if err != nil {
		apiShared.BadRequest(c, "跨集群服务 ID 无效")
		return
	}
	item, err := h.store.GetCrossClusterService(clusterID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "跨集群服务不存在")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "读取跨集群服务失败")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Second)
	defer cancel()
	client, err := h.client(ctx, item.SourceClusterID)
	if err == nil {
		err = h.removeSourceResources(ctx, client, item)
	}
	if err == nil {
		var server *model.Server
		server, err = h.store.GetServer(item.TargetServerID)
		if err == nil {
			err = h.removeGateway(ctx, server, item.ID)
		}
	}
	if err != nil {
		message := "删除跨集群服务失败：" + err.Error()
		_ = h.store.UpdateCrossClusterServiceState(id, "failed", message)
		_ = h.store.RecordCrossClusterServiceTask(item.SourceClusterID, item.TargetServerID, "cross-delete", message, false)
		apiShared.BadRequest(c, message)
		return
	}
	if err := h.store.DeleteCrossClusterService(id); err != nil {
		apiShared.InternalError(c, "资源已删除，但清理记录失败")
		return
	}
	_ = h.store.RecordCrossClusterServiceTask(item.SourceClusterID, item.TargetServerID, "cross-delete", "已删除 "+item.SourceNamespace+"/"+item.SourceName, true)
	apiShared.Success(c, gin.H{"deleted": true})
}

func (h *CrossServiceHandler) Check(c *gin.Context) {
	clusterID, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "集群 ID 无效")
		return
	}
	id, err := apiShared.ParsePositiveID(c.Param("serviceID"))
	if err != nil {
		apiShared.BadRequest(c, "跨集群服务 ID 无效")
		return
	}
	item, err := h.store.GetCrossClusterService(clusterID, id)
	if err != nil {
		apiShared.NotFound(c, "跨集群服务不存在")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Second)
	defer cancel()
	client, err := h.client(ctx, item.SourceClusterID)
	if err != nil {
		apiShared.Success(c, gin.H{"status": "unavailable", "message": "来源集群 Agent 不可用"})
		return
	}
	service, err := client.Clientset.CoreV1().Services(item.SourceNamespace).Get(ctx, item.SourceName, metav1.GetOptions{})
	if err != nil || service.Annotations["cylism.io/cross-service-id"] != crossServiceOwner(item.ID) {
		apiShared.Success(c, gin.H{"status": "unavailable", "message": "来源 Service 不存在或已被修改"})
		return
	}
	slice, err := client.Clientset.DiscoveryV1().EndpointSlices(item.SourceNamespace).Get(ctx, item.SourceName+"-cross", metav1.GetOptions{})
	if err != nil || slice.Annotations["cylism.io/cross-service-id"] != crossServiceOwner(item.ID) {
		apiShared.Success(c, gin.H{"status": "unavailable", "message": "来源 EndpointSlice 不存在或已被修改"})
		return
	}
	if err := h.probeGateway(ctx, item); err != nil {
		apiShared.Success(c, gin.H{"status": "unavailable", "message": err.Error()})
		return
	}
	apiShared.Success(c, gin.H{"status": "node_reachable", "message": "来源节点均可连接目标入口；Pod 内访问尚未验证"})
}
