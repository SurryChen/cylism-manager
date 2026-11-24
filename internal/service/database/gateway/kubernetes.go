package gateway

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const namespace = "cylism-databases"
const ownerLabel = "cylism.io/database-gateway"
const probeLabel = "cylism.io/database-gateway-probe"

func pointer[T any](value T) *T { return &value }
func labels(id uint) map[string]string {
	return map[string]string{k8s.ManagedByLabel: k8s.ManagedByValue, ownerLabel: fmt.Sprint(id)}
}
func owns(values map[string]string, id uint) bool {
	return values[k8s.ManagedByLabel] == k8s.ManagedByValue && values[ownerLabel] == fmt.Sprint(id)
}

func ApplyService(ctx context.Context, client *k8s.Client, config *model.DatabaseGateway) error {
	if client == nil || client.Clientset == nil {
		return fmt.Errorf("集群客户端不可用")
	}
	ns := client.Clientset.CoreV1().Namespaces()
	if _, err := ns.Get(ctx, namespace, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		if _, err = ns.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	} else if err != nil {
		return err
	}
	name := Name(config.ID)
	services := client.Clientset.CoreV1().Services(namespace)
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels(config.ID)}, Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "mysql", Protocol: corev1.ProtocolTCP, Port: 3306, TargetPort: intstr.FromInt(config.ListenPort)}}}}
	existing, err := services.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		service, err = services.Create(ctx, service, metav1.CreateOptions{})
	} else if err == nil {
		if !owns(existing.Labels, config.ID) {
			return fmt.Errorf("同名 Service 不属于此数据库网关")
		}
		if existing.Spec.Selector != nil || existing.Spec.Type == corev1.ServiceTypeExternalName || existing.Spec.Type == corev1.ServiceTypeNodePort || existing.Spec.Type == corev1.ServiceTypeLoadBalancer {
			return fmt.Errorf("已有网关 Service 类型或选择器异常")
		}
		service.ResourceVersion, service.Spec.ClusterIP, service.Spec.ClusterIPs = existing.ResourceVersion, existing.Spec.ClusterIP, existing.Spec.ClusterIPs
		service.Spec.IPFamilies, service.Spec.IPFamilyPolicy = existing.Spec.IPFamilies, existing.Spec.IPFamilyPolicy
		service, err = services.Update(ctx, service, metav1.UpdateOptions{})
	}
	if err != nil {
		return err
	}
	sliceLabels := labels(config.ID)
	sliceLabels[discoveryv1.LabelServiceName], sliceLabels[discoveryv1.LabelManagedBy] = name, "cylism-manager"
	slice := &discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: sliceLabels, OwnerReferences: []metav1.OwnerReference{{APIVersion: "v1", Kind: "Service", Name: name, UID: service.UID}}}, AddressType: discoveryv1.AddressTypeIPv4, Ports: []discoveryv1.EndpointPort{{Name: pointer("mysql"), Protocol: pointer(corev1.ProtocolTCP), Port: pointer(int32(config.ListenPort))}}, Endpoints: []discoveryv1.Endpoint{{Addresses: []string{config.ListenIP}, Conditions: discoveryv1.EndpointConditions{Ready: pointer(true)}}}}
	slices := client.Clientset.DiscoveryV1().EndpointSlices(namespace)
	current, err := slices.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = slices.Create(ctx, slice, metav1.CreateOptions{})
	} else if err == nil {
		if !owns(current.Labels, config.ID) {
			return fmt.Errorf("同名 EndpointSlice 不属于此数据库网关")
		}
		slice.ResourceVersion = current.ResourceVersion
		_, err = slices.Update(ctx, slice, metav1.UpdateOptions{})
	}
	return err
}

func DeleteService(ctx context.Context, client *k8s.Client, config *model.DatabaseGateway) error {
	if client == nil || client.Clientset == nil {
		return fmt.Errorf("集群客户端不可用")
	}
	slices := client.Clientset.DiscoveryV1().EndpointSlices(namespace)
	if item, err := slices.Get(ctx, Name(config.ID), metav1.GetOptions{}); err == nil {
		if !owns(item.Labels, config.ID) {
			return fmt.Errorf("EndpointSlice 不属于此网关，拒绝删除")
		}
		if err := slices.Delete(ctx, item.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &item.UID}}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	services := client.Clientset.CoreV1().Services(namespace)
	if item, err := services.Get(ctx, Name(config.ID), metav1.GetOptions{}); err == nil {
		if !owns(item.Labels, config.ID) {
			return fmt.Errorf("Service 不属于此网关，拒绝删除")
		}
		return services.Delete(ctx, item.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &item.UID}})
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

const probeSource = `import os, socket, time
for attempt in range(3):
    with socket.create_connection((os.environ['DB_HOST'], 3306), timeout=5) as connection:
        greeting = b''
        while len(greeting) < 5:
            data = connection.recv(5-len(greeting))
            if not data:
                raise RuntimeError('gateway closed connection; check source allowlist and private target')
            greeting += data
        if greeting[4] != 10:
            raise RuntimeError('endpoint did not send a MySQL greeting')
    print('MySQL greeting received %d/3' % (attempt+1), flush=True)
    time.sleep(0.2)
`

// Probe checks the Service from an unprivileged Pod on every ready, schedulable
// node. It proves DNS, routing, ACL and MySQL greeting, not login or TLS identity.
func Probe(ctx context.Context, client *k8s.Client, config *model.DatabaseGateway, log func(string) error) error {
	nodes, err := client.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}
	checked := 0
	for _, node := range nodes.Items {
		if node.Spec.Unschedulable {
			continue
		}
		ready := false
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				ready = true
			}
		}
		if !ready {
			continue
		}
		checked++
		if err := log(fmt.Sprintf("正在验证节点 %s：通过 Service 连续读取三次 MySQL 握手", node.Name)); err != nil {
			return err
		}
		if err := probeNode(ctx, client, config, node.Name); err != nil {
			return fmt.Errorf("节点 %s 测试失败：%w", node.Name, err)
		}
		if err := log("节点 " + node.Name + "：3/3 次通过"); err != nil {
			return err
		}
	}
	if checked == 0 {
		return fmt.Errorf("集群没有 Ready 且可调度的节点")
	}
	return nil
}

func probeNode(parent context.Context, client *k8s.Client, config *model.DatabaseGateway, node string) error {
	ctx, cancel := context.WithTimeout(parent, 3*time.Minute)
	defer cancel()
	pods := client.Clientset.CoreV1().Pods(namespace)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: Name(config.ID) + "-probe-", Namespace: namespace, Labels: labels(config.ID),
		},
		Spec: corev1.PodSpec{
			NodeName: node, RestartPolicy: corev1.RestartPolicyNever,
			ActiveDeadlineSeconds: pointer(int64(150)), AutomountServiceAccountToken: pointer(false),
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: pointer(true), RunAsUser: pointer(int64(65534)),
				SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			},
			Containers: []corev1.Container{{
				Name: "probe", Image: config.ProbeImage, Command: []string{"python3", "-c", probeSource},
				Env: []corev1.EnvVar{{Name: "DB_HOST", Value: ServiceHost(config.ID)}},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: pointer(false), ReadOnlyRootFilesystem: pointer(true),
					Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m"), corev1.ResourceMemory: resource.MustParse("16Mi")},
					Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("48Mi")},
				},
			}},
		},
	}
	pod.Labels[probeLabel] = "true"
	pod.Spec.DNSConfig = &corev1.PodDNSConfig{Options: []corev1.PodDNSConfigOption{{Name: "ndots", Value: pointer("1")}}}
	created, err := pods.Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = pods.Delete(cleanup, created.Name, metav1.DeleteOptions{GracePeriodSeconds: pointer(int64(0)), Preconditions: &metav1.Preconditions{UID: &created.UID}})
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		current, err := pods.Get(ctx, created.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if current.Status.Phase == corev1.PodSucceeded {
			return nil
		}
		if current.Status.Phase == corev1.PodFailed {
			limit := int64(4096)
			data, _ := pods.GetLogs(created.Name, &corev1.PodLogOptions{Container: "probe", LimitBytes: &limit}).Do(ctx).Raw()
			return fmt.Errorf("%s %s", current.Status.Reason, strings.TrimSpace(string(data)))
		}
		for _, container := range current.Status.ContainerStatuses {
			if waiting := container.State.Waiting; waiting != nil && (waiting.Reason == "ErrImagePull" || waiting.Reason == "ImagePullBackOff" || waiting.Reason == "CreateContainerConfigError") {
				return fmt.Errorf("诊断 Pod %s：%s", waiting.Reason, waiting.Message)
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("诊断 Pod 等待超时；请检查镜像、节点与网络：%w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// CleanupProbes removes only this gateway's diagnostics left by a hard process
// crash. The task executor serializes probes before invoking this cleanup.
func CleanupProbes(ctx context.Context, client *k8s.Client, config *model.DatabaseGateway) error {
	pods := client.Clientset.CoreV1().Pods(namespace)
	items, err := pods.List(ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%d,%s=true", ownerLabel, config.ID, probeLabel)})
	if err != nil {
		return err
	}
	for _, item := range items.Items {
		if !owns(item.Labels, config.ID) {
			continue
		}
		if err := pods.Delete(ctx, item.Name, metav1.DeleteOptions{GracePeriodSeconds: pointer(int64(0)), Preconditions: &metav1.Preconditions{UID: &item.UID}}); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}
