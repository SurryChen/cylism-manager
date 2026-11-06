package database

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const Namespace = "cylism-databases"

func ResourceName(id uint) string { return fmt.Sprintf("cylism-mysql-%d", id) }

func Apply(ctx context.Context, client *k8s.Client, instance *model.DatabaseInstance, rootPassword, password string) error {
	if client == nil || client.Clientset == nil {
		return fmt.Errorf("集群 Kubernetes 客户端不可用")
	}
	name := ResourceName(instance.ID)
	labels := map[string]string{k8s.ManagedByLabel: k8s.ManagedByValue, "app.kubernetes.io/name": name}
	namespaces := client.Clientset.CoreV1().Namespaces()
	if _, err := namespaces.Get(ctx, Namespace, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		if _, err = namespaces.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: Namespace}}, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("创建数据库命名空间: %w", err)
		}
	} else if err != nil {
		return err
	}
	secretData := map[string]string{"root-password": rootPassword, "password": password}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: Namespace, Labels: labels}, Type: corev1.SecretTypeOpaque, StringData: secretData}
	if existing, err := client.Clientset.CoreV1().Secrets(Namespace).Get(ctx, name, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		if _, err = client.Clientset.CoreV1().Secrets(Namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("创建数据库密钥: %w", err)
		}
	} else if err != nil {
		return err
	} else if existing.Labels[k8s.ManagedByLabel] != k8s.ManagedByValue {
		return fmt.Errorf("同名数据库密钥不受平台管理")
	} else {
		secret.ResourceVersion = existing.ResourceVersion
		if _, err = client.Clientset.CoreV1().Secrets(Namespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	service := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: Namespace, Labels: labels}, Spec: corev1.ServiceSpec{Selector: labels, Ports: []corev1.ServicePort{{Name: "mysql", Port: 3306, TargetPort: intstr.FromInt(3306)}}}}
	if existing, err := client.Clientset.CoreV1().Services(Namespace).Get(ctx, name, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		if _, err = client.Clientset.CoreV1().Services(Namespace).Create(ctx, service, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("创建数据库 Service: %w", err)
		}
	} else if err != nil {
		return err
	} else if existing.Labels[k8s.ManagedByLabel] != k8s.ManagedByValue {
		return fmt.Errorf("同名数据库 Service 不受平台管理")
	} else {
		service.ResourceVersion = existing.ResourceVersion
		service.Spec.ClusterIP = existing.Spec.ClusterIP
		service.Spec.ClusterIPs = existing.Spec.ClusterIPs
		if _, err = client.Clientset.CoreV1().Services(Namespace).Update(ctx, service, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	storage, err := resource.ParseQuantity(instance.StorageSize)
	if err != nil {
		return fmt.Errorf("存储容量无效: %w", err)
	}
	claim := corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "data", Labels: labels}, Spec: corev1.PersistentVolumeClaimSpec{AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}, Resources: corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: storage}}}}
	if instance.StorageClass != "" {
		claim.Spec.StorageClassName = &instance.StorageClass
	}
	replicas := int32(1)
	container := corev1.Container{Name: "mysql", Image: "mysql:8.4", Ports: []corev1.ContainerPort{{ContainerPort: 3306, Name: "mysql"}}, Env: []corev1.EnvVar{{Name: "MYSQL_ROOT_PASSWORD", ValueFrom: secretKey(name, "root-password")}, {Name: "MYSQL_ROOT_HOST", Value: "%"}}, VolumeMounts: []corev1.VolumeMount{{Name: "data", MountPath: "/var/lib/mysql"}}, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m"), corev1.ResourceMemory: resource.MustParse("512Mi")}, Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("2Gi")}}, ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt(3306)}}, PeriodSeconds: 10}}
	if instance.DatabaseName != "" && instance.Username != "" && password != "" {
		container.Env = append(container.Env, corev1.EnvVar{Name: "MYSQL_DATABASE", Value: instance.DatabaseName}, corev1.EnvVar{Name: "MYSQL_USER", Value: instance.Username}, corev1.EnvVar{Name: "MYSQL_PASSWORD", ValueFrom: secretKey(name, "password")})
	}
	serverID := instance.ID + 10000
	container.Args = []string{"--server-id=" + strconv.FormatUint(uint64(serverID), 10), "--log-bin=mysql-bin", "--gtid-mode=ON", "--enforce-gtid-consistency=ON", "--local-infile=ON", "--binlog-expire-logs-seconds=604800"}
	if instance.CPULimit != "" && instance.MemoryLimit != "" {
		cpu, cpuErr := resource.ParseQuantity(instance.CPULimit)
		memory, memoryErr := resource.ParseQuantity(instance.MemoryLimit)
		if cpuErr != nil || memoryErr != nil || cpu.Sign() <= 0 || memory.Sign() <= 0 {
			return fmt.Errorf("MySQL CPU 或内存资源限制无效")
		}
		container.Resources = corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: cpu, corev1.ResourceMemory: memory}, Limits: corev1.ResourceList{corev1.ResourceCPU: cpu, corev1.ResourceMemory: memory}}
	}
	for _, line := range strings.Split(EffectiveMyCNF("", instance.MemoryLimit), "\n") {
		if key, value, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			container.Args = append(container.Args, "--"+strings.ReplaceAll(key, "_", "-")+"="+value)
		}
	}
	podSpec := corev1.PodSpec{Containers: []corev1.Container{container}, Tolerations: []corev1.Toleration{{Key: databaseNoScheduleTaintKey, Value: "true", Operator: corev1.TolerationOpEqual, Effect: corev1.TaintEffectNoSchedule}}}
	if instance.NodeName != "" {
		podSpec.NodeSelector = map[string]string{corev1.LabelHostname: instance.NodeName}
	}
	if strings.TrimSpace(rootPassword) == "" {
		return fmt.Errorf("MySQL 管理密码不能为空")
	}
	stateful := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: Namespace, Labels: labels}, Spec: appsv1.StatefulSetSpec{ServiceName: name, Replicas: &replicas, Selector: &metav1.LabelSelector{MatchLabels: labels}, VolumeClaimTemplates: []corev1.PersistentVolumeClaim{claim}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: podSpec}}}
	if instance.Image != "" {
		stateful.Spec.Template.Spec.Containers[0].Image = instance.Image
	}
	sets := client.Clientset.AppsV1().StatefulSets(Namespace)
	if existing, err := sets.Get(ctx, name, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		if _, err = sets.Create(ctx, stateful, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("创建 MySQL StatefulSet: %w", err)
		}
	} else if err != nil {
		return err
	} else if existing.Labels[k8s.ManagedByLabel] != k8s.ManagedByValue {
		return fmt.Errorf("同名 StatefulSet 不受平台管理")
	} else {
		// Storage claims cannot be resized by replacing the StatefulSet template.
		stateful.Spec.VolumeClaimTemplates = existing.Spec.VolumeClaimTemplates
		if len(existing.Spec.Template.Spec.Containers) > 0 && existing.Spec.Template.Spec.Containers[0].Image != stateful.Spec.Template.Spec.Containers[0].Image {
			return fmt.Errorf("已有 MySQL 实例不支持通过部署任务更换镜像")
		}
		stateful.ResourceVersion = existing.ResourceVersion
		if _, err = sets.Update(ctx, stateful, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func secretKey(name, key string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: name}, Key: key}}
}

func Status(ctx context.Context, client *k8s.Client, instance *model.DatabaseInstance) (string, error) {
	if client == nil || client.Clientset == nil {
		return "unknown", fmt.Errorf("集群 Kubernetes 客户端不可用")
	}
	set, err := client.Clientset.AppsV1().StatefulSets(Namespace).Get(ctx, ResourceName(instance.ID), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "missing", nil
	}
	if err != nil {
		return "unknown", err
	}
	if set.Status.ReadyReplicas == 1 {
		return "ready", nil
	}
	return "starting", nil
}

func Delete(ctx context.Context, client *k8s.Client, instance *model.DatabaseInstance) error {
	if client == nil || client.Clientset == nil {
		return fmt.Errorf("集群 Kubernetes 客户端不可用")
	}
	name := ResourceName(instance.ID)
	for _, kind := range []struct {
		name   string
		get    func() (map[string]string, error)
		remove func() error
	}{
		{"StatefulSet", func() (map[string]string, error) {
			o, e := client.Clientset.AppsV1().StatefulSets(Namespace).Get(ctx, name, metav1.GetOptions{})
			if e != nil {
				return nil, e
			}
			return o.Labels, nil
		}, func() error {
			return client.Clientset.AppsV1().StatefulSets(Namespace).Delete(ctx, name, metav1.DeleteOptions{})
		}},
		{"Service", func() (map[string]string, error) {
			o, e := client.Clientset.CoreV1().Services(Namespace).Get(ctx, name, metav1.GetOptions{})
			if e != nil {
				return nil, e
			}
			return o.Labels, nil
		}, func() error {
			return client.Clientset.CoreV1().Services(Namespace).Delete(ctx, name, metav1.DeleteOptions{})
		}},
		{"Secret", func() (map[string]string, error) {
			o, e := client.Clientset.CoreV1().Secrets(Namespace).Get(ctx, name, metav1.GetOptions{})
			if e != nil {
				return nil, e
			}
			return o.Labels, nil
		}, func() error {
			return client.Clientset.CoreV1().Secrets(Namespace).Delete(ctx, name, metav1.DeleteOptions{})
		}},
	} {
		labels, err := kind.get()
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		if labels[k8s.ManagedByLabel] != k8s.ManagedByValue {
			return fmt.Errorf("%s 不受平台管理", kind.name)
		}
		if err := kind.remove(); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	// StatefulSet PVCs are intentionally retained for recovery.
	return nil
}

func PVCName(id uint) string     { return "data-" + ResourceName(id) + "-0" }
func ServiceHost(id uint) string { return ResourceName(id) + "." + Namespace + ".svc.cluster.local" }
func ServicePort() string        { return strconv.Itoa(3306) }
