package k8s

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadRef 工作负载引用
type WorkloadRef struct {
	Kind    string `json:"kind"` // Deployment / StatefulSet / DaemonSet
	Name    string `json:"name"`
	RefType string `json:"ref_type"` // volume / env / envFrom
}

// ConfigMapInfo ConfigMap 展示信息
type ConfigMapInfo struct {
	Name      string        `json:"name"`
	Namespace string        `json:"namespace"`
	Keys      []string      `json:"keys"`
	KeysCount int           `json:"keys_count"`
	UsedBy    []WorkloadRef `json:"used_by"`
	Age       string        `json:"age"`
}

// ConfigMapDetail ConfigMap 详情
type ConfigMapDetail struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Data      map[string]string `json:"data"`
	UsedBy    []WorkloadRef     `json:"used_by"`
	Age       string            `json:"age"`
}

// SecretInfo Secret 展示信息（不包含 value）
type SecretInfo struct {
	Name      string        `json:"name"`
	Namespace string        `json:"namespace"`
	Type      string        `json:"type"`
	Keys      []string      `json:"keys"`
	KeysCount int           `json:"keys_count"`
	UsedBy    []WorkloadRef `json:"used_by"`
	Age       string        `json:"age"`
}

// SecretDetail Secret 详情（含 base64 value）
type SecretDetail struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Type      string            `json:"type"`
	Data      map[string]string `json:"data"` // base64 encoded values
	UsedBy    []WorkloadRef     `json:"used_by"`
	Age       string            `json:"age"`
}

// ConfigMapMutation is the writable subset of a ConfigMap exposed by the
// platform resource editor. Metadata and binary data remain cluster-managed.
type ConfigMapMutation struct {
	Namespace string
	Name      string
	Data      map[string]string
}

// OpaqueSecretMutation only permits application-owned Opaque Secrets. Empty
// values on update preserve the existing value so callers never need to read
// secret material to edit a resource.
type OpaqueSecretMutation struct {
	Namespace string
	Name      string
	Data      map[string]string
}

// ListConfigMaps 列出所有 ConfigMap
func (c *Client) ListConfigMapsContext(ctx context.Context, ns string) ([]ConfigMapInfo, error) {
	return c.listConfigMapsContext(ctx, ns, true)
}

// ListConfigMapsMetadata lists only resource metadata and keys. It avoids
// workload reference scans for selectors and inventory pages.
func (c *Client) ListConfigMapsMetadataContext(ctx context.Context, ns string) ([]ConfigMapInfo, error) {
	return c.listConfigMapsContext(ctx, ns, false)
}

func (c *Client) listConfigMapsContext(ctx context.Context, ns string, includeUsage bool) ([]ConfigMapInfo, error) {
	var list *corev1.ConfigMapList
	var err error
	if ns == "" {
		list, err = c.Clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	} else {
		list, err = c.Clientset.CoreV1().ConfigMaps(ns).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("list configmaps: %w", err)
	}

	result := make([]ConfigMapInfo, 0, len(list.Items))
	refsByResource := make(map[configReferenceKey][]WorkloadRef)
	if includeUsage {
		refsByResource = c.buildConfigReferenceIndex(ctx, referenceNamespacesFromConfigMaps(list.Items), "ConfigMap")
	}
	for _, cm := range list.Items {
		keys := make([]string, 0, len(cm.Data))
		for k := range cm.Data {
			keys = append(keys, k)
		}
		result = append(result, ConfigMapInfo{
			Name:      cm.Name,
			Namespace: cm.Namespace,
			Keys:      keys,
			KeysCount: len(cm.Data),
			UsedBy:    refsByResource[configReferenceKey{Namespace: cm.Namespace, Name: cm.Name}],
			Age:       timeAgo(cm.CreationTimestamp.Time),
		})
	}
	return result, nil
}

// GetConfigMap 获取 ConfigMap 详情
func (c *Client) GetConfigMapContext(ctx context.Context, namespace, name string) (*ConfigMapDetail, error) {
	cm, err := c.Clientset.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get configmap %s/%s: %w", namespace, name, err)
	}
	return &ConfigMapDetail{
		Name:      cm.Name,
		Namespace: cm.Namespace,
		Data:      cm.Data,
		UsedBy:    c.findConfigRefs(ctx, namespace, name, "ConfigMap"),
		Age:       timeAgo(cm.CreationTimestamp.Time),
	}, nil
}

func (c *Client) CreateConfigMapContext(ctx context.Context, request ConfigMapMutation) (*ConfigMapDetail, error) {
	created, err := c.Clientset.CoreV1().ConfigMaps(request.Namespace).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: request.Name, Namespace: request.Namespace},
		Data:       request.Data,
	}, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create configmap %s/%s: %w", request.Namespace, request.Name, err)
	}
	return &ConfigMapDetail{Name: created.Name, Namespace: created.Namespace, Data: created.Data, Age: timeAgo(created.CreationTimestamp.Time)}, nil
}

func (c *Client) UpdateConfigMapContext(ctx context.Context, request ConfigMapMutation) (*ConfigMapDetail, error) {
	configMap, err := c.Clientset.CoreV1().ConfigMaps(request.Namespace).Get(ctx, request.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get configmap %s/%s: %w", request.Namespace, request.Name, err)
	}
	configMap.Data = request.Data
	updated, err := c.Clientset.CoreV1().ConfigMaps(request.Namespace).Update(ctx, configMap, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("update configmap %s/%s: %w", request.Namespace, request.Name, err)
	}
	return &ConfigMapDetail{Name: updated.Name, Namespace: updated.Namespace, Data: updated.Data, UsedBy: c.findConfigRefs(ctx, request.Namespace, request.Name, "ConfigMap"), Age: timeAgo(updated.CreationTimestamp.Time)}, nil
}

func (c *Client) DeleteConfigMapContext(ctx context.Context, namespace, name string) error {
	if err := c.Clientset.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("delete configmap %s/%s: %w", namespace, name, err)
	}
	return nil
}

// ListSecrets 列出所有 Secret（不返回 value）
func (c *Client) ListSecretsContext(ctx context.Context, ns string) ([]SecretInfo, error) {
	return c.listSecretsContext(ctx, ns, true)
}

// ListSecretsMetadata lists only resource metadata and keys. Secret values
// and workload scans are intentionally excluded from this inventory path.
func (c *Client) ListSecretsMetadataContext(ctx context.Context, ns string) ([]SecretInfo, error) {
	return c.listSecretsContext(ctx, ns, false)
}

func (c *Client) listSecretsContext(ctx context.Context, ns string, includeUsage bool) ([]SecretInfo, error) {
	var list *corev1.SecretList
	var err error
	if ns == "" {
		list, err = c.Clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	} else {
		list, err = c.Clientset.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}

	result := make([]SecretInfo, 0, len(list.Items))
	refsByResource := make(map[configReferenceKey][]WorkloadRef)
	if includeUsage {
		refsByResource = c.buildConfigReferenceIndex(ctx, referenceNamespacesFromSecrets(list.Items), "Secret")
	}
	for _, s := range list.Items {
		keys := make([]string, 0, len(s.Data))
		for k := range s.Data {
			keys = append(keys, k)
		}
		result = append(result, SecretInfo{
			Name:      s.Name,
			Namespace: s.Namespace,
			Type:      string(s.Type),
			Keys:      keys,
			KeysCount: len(s.Data),
			UsedBy:    refsByResource[configReferenceKey{Namespace: s.Namespace, Name: s.Name}],
			Age:       timeAgo(s.CreationTimestamp.Time),
		})
	}
	return result, nil
}

type configReferenceKey struct {
	Namespace string
	Name      string
}

func referenceNamespacesFromConfigMaps(items []corev1.ConfigMap) []string {
	namespaces := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if _, exists := seen[item.Namespace]; exists {
			continue
		}
		seen[item.Namespace] = struct{}{}
		namespaces = append(namespaces, item.Namespace)
	}
	return namespaces
}

func referenceNamespacesFromSecrets(items []corev1.Secret) []string {
	namespaces := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if _, exists := seen[item.Namespace]; exists {
			continue
		}
		seen[item.Namespace] = struct{}{}
		namespaces = append(namespaces, item.Namespace)
	}
	return namespaces
}

// buildConfigReferenceIndex scans each workload type once per namespace instead of once per resource.
func (c *Client) buildConfigReferenceIndex(ctx context.Context, namespaces []string, kind string) map[configReferenceKey][]WorkloadRef {
	refsByResource := make(map[configReferenceKey][]WorkloadRef)
	for _, namespace := range namespaces {
		if deploys, err := c.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{}); err == nil {
			for _, deployment := range deploys.Items {
				indexWorkloadConfigRefs(deployment.Namespace, deployment.Name, "Deployment", deployment.Spec.Template.Spec, kind, refsByResource)
			}
		}
		if statefulSets, err := c.Clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{}); err == nil {
			for _, statefulSet := range statefulSets.Items {
				indexWorkloadConfigRefs(statefulSet.Namespace, statefulSet.Name, "StatefulSet", statefulSet.Spec.Template.Spec, kind, refsByResource)
			}
		}
		if daemonSets, err := c.Clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{}); err == nil {
			for _, daemonSet := range daemonSets.Items {
				indexWorkloadConfigRefs(daemonSet.Namespace, daemonSet.Name, "DaemonSet", daemonSet.Spec.Template.Spec, kind, refsByResource)
			}
		}
	}
	return refsByResource
}

func indexWorkloadConfigRefs(namespace, workloadName, workloadKind string, spec corev1.PodSpec, kind string, refsByResource map[configReferenceKey][]WorkloadRef) {
	appendRef := func(name, refType string) {
		if name == "" {
			return
		}
		key := configReferenceKey{Namespace: namespace, Name: name}
		refsByResource[key] = append(refsByResource[key], WorkloadRef{Kind: workloadKind, Name: workloadName, RefType: refType})
	}
	for _, container := range spec.Containers {
		for _, envFrom := range container.EnvFrom {
			if kind == "ConfigMap" && envFrom.ConfigMapRef != nil {
				appendRef(envFrom.ConfigMapRef.Name, "envFrom")
			}
			if kind == "Secret" && envFrom.SecretRef != nil {
				appendRef(envFrom.SecretRef.Name, "envFrom")
			}
		}
		for _, env := range container.Env {
			if env.ValueFrom == nil {
				continue
			}
			if kind == "ConfigMap" && env.ValueFrom.ConfigMapKeyRef != nil {
				appendRef(env.ValueFrom.ConfigMapKeyRef.Name, "env")
			}
			if kind == "Secret" && env.ValueFrom.SecretKeyRef != nil {
				appendRef(env.ValueFrom.SecretKeyRef.Name, "env")
			}
		}
	}
	for _, volume := range spec.Volumes {
		if kind == "ConfigMap" && volume.ConfigMap != nil {
			appendRef(volume.ConfigMap.Name, "volume")
		}
		if kind == "Secret" && volume.Secret != nil {
			appendRef(volume.Secret.SecretName, "volume")
		}
	}
}

// GetSecret 获取 Secret 详情（含 base64 value）
func (c *Client) GetSecretContext(ctx context.Context, namespace, name string) (*SecretDetail, error) {
	s, err := c.Clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get secret %s/%s: %w", namespace, name, err)
	}
	data := make(map[string]string)
	for k, v := range s.Data {
		data[k] = string(v)
	}
	return &SecretDetail{
		Name:      s.Name,
		Namespace: s.Namespace,
		Type:      string(s.Type),
		Data:      data,
		UsedBy:    c.findConfigRefs(ctx, namespace, name, "Secret"),
		Age:       timeAgo(s.CreationTimestamp.Time),
	}, nil
}

// GetSecretDataContext exposes only Secret data to internal services that
// need credential values, without coupling them to the typed clientset.
func (c *Client) GetSecretDataContext(ctx context.Context, namespace, name string) (map[string][]byte, error) {
	if c == nil || c.Clientset == nil {
		return nil, fmt.Errorf("Kubernetes 客户端未初始化")
	}
	secret, err := c.Clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	data := make(map[string][]byte, len(secret.Data))
	for key, value := range secret.Data {
		data[key] = append([]byte(nil), value...)
	}
	return data, nil
}

func (c *Client) CreateOpaqueSecretContext(ctx context.Context, request OpaqueSecretMutation) (*SecretInfo, error) {
	data := make(map[string][]byte, len(request.Data))
	for key, value := range request.Data {
		data[key] = []byte(value)
	}
	created, err := c.Clientset.CoreV1().Secrets(request.Namespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: request.Name, Namespace: request.Namespace},
		Type:       corev1.SecretTypeOpaque,
		Data:       data,
	}, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create secret %s/%s: %w", request.Namespace, request.Name, err)
	}
	return secretInfoFromObject(*created, nil), nil
}

func (c *Client) UpdateOpaqueSecretContext(ctx context.Context, request OpaqueSecretMutation) (*SecretInfo, error) {
	secret, err := c.Clientset.CoreV1().Secrets(request.Namespace).Get(ctx, request.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get secret %s/%s: %w", request.Namespace, request.Name, err)
	}
	if secret.Type != corev1.SecretTypeOpaque {
		return nil, fmt.Errorf("secret %s/%s is not an Opaque Secret", request.Namespace, request.Name)
	}
	data := make(map[string][]byte, len(request.Data))
	for key, value := range request.Data {
		if value == "" {
			if existing, exists := secret.Data[key]; exists {
				data[key] = existing
				continue
			}
		}
		data[key] = []byte(value)
	}
	secret.Data = data
	updated, err := c.Clientset.CoreV1().Secrets(request.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("update secret %s/%s: %w", request.Namespace, request.Name, err)
	}
	return secretInfoFromObject(*updated, c.findConfigRefs(ctx, request.Namespace, request.Name, "Secret")), nil
}

func (c *Client) DeleteOpaqueSecretContext(ctx context.Context, namespace, name string) error {
	secret, err := c.Clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get secret %s/%s: %w", namespace, name, err)
	}
	if secret.Type != corev1.SecretTypeOpaque {
		return fmt.Errorf("secret %s/%s is not an Opaque Secret", namespace, name)
	}
	if err := c.Clientset.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("delete secret %s/%s: %w", namespace, name, err)
	}
	return nil
}

func secretInfoFromObject(secret corev1.Secret, usedBy []WorkloadRef) *SecretInfo {
	keys := make([]string, 0, len(secret.Data))
	for key := range secret.Data {
		keys = append(keys, key)
	}
	return &SecretInfo{Name: secret.Name, Namespace: secret.Namespace, Type: string(secret.Type), Keys: keys, KeysCount: len(keys), UsedBy: usedBy, Age: timeAgo(secret.CreationTimestamp.Time)}
}

// findConfigRefs 查找引用指定 ConfigMap/Secret 的工作负载
func (c *Client) findConfigRefs(ctx context.Context, ns, name string, kind string) []WorkloadRef {
	var refs []WorkloadRef

	// 查 Deployment
	deploys, err := c.Clientset.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		refs = append(refs, findRefsInDeployments(deploys.Items, name, kind)...)
	}

	// 查 StatefulSet
	sts, err := c.Clientset.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		refs = append(refs, findRefsInStatefulSets(sts.Items, name, kind)...)
	}

	// 查 DaemonSet
	ds, err := c.Clientset.AppsV1().DaemonSets(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		refs = append(refs, findRefsInDaemonSets(ds.Items, name, kind)...)
	}

	return refs
}

func findRefsInDeployments(items []appsv1.Deployment, name, kind string) []WorkloadRef {
	var refs []WorkloadRef
	for _, d := range items {
		for _, container := range d.Spec.Template.Spec.Containers {
			// envFrom
			for _, envFrom := range container.EnvFrom {
				if envFrom.ConfigMapRef != nil && kind == "ConfigMap" && envFrom.ConfigMapRef.Name == name {
					refs = append(refs, WorkloadRef{Kind: "Deployment", Name: d.Name, RefType: "envFrom"})
				}
				if envFrom.SecretRef != nil && kind == "Secret" && envFrom.SecretRef.Name == name {
					refs = append(refs, WorkloadRef{Kind: "Deployment", Name: d.Name, RefType: "envFrom"})
				}
			}
			// env valueFrom
			for _, env := range container.Env {
				if env.ValueFrom != nil {
					if env.ValueFrom.ConfigMapKeyRef != nil && kind == "ConfigMap" && env.ValueFrom.ConfigMapKeyRef.Name == name {
						refs = append(refs, WorkloadRef{Kind: "Deployment", Name: d.Name, RefType: "env"})
					}
					if env.ValueFrom.SecretKeyRef != nil && kind == "Secret" && env.ValueFrom.SecretKeyRef.Name == name {
						refs = append(refs, WorkloadRef{Kind: "Deployment", Name: d.Name, RefType: "env"})
					}
				}
			}
		}
		// volumes
		for _, vol := range d.Spec.Template.Spec.Volumes {
			if vol.ConfigMap != nil && kind == "ConfigMap" && vol.ConfigMap.Name == name {
				refs = append(refs, WorkloadRef{Kind: "Deployment", Name: d.Name, RefType: "volume"})
			}
			if vol.Secret != nil && kind == "Secret" && vol.Secret.SecretName == name {
				refs = append(refs, WorkloadRef{Kind: "Deployment", Name: d.Name, RefType: "volume"})
			}
		}
	}
	return refs
}

func findRefsInStatefulSets(items []appsv1.StatefulSet, name, kind string) []WorkloadRef {
	var refs []WorkloadRef
	for _, s := range items {
		for _, container := range s.Spec.Template.Spec.Containers {
			for _, envFrom := range container.EnvFrom {
				if envFrom.ConfigMapRef != nil && kind == "ConfigMap" && envFrom.ConfigMapRef.Name == name {
					refs = append(refs, WorkloadRef{Kind: "StatefulSet", Name: s.Name, RefType: "envFrom"})
				}
				if envFrom.SecretRef != nil && kind == "Secret" && envFrom.SecretRef.Name == name {
					refs = append(refs, WorkloadRef{Kind: "StatefulSet", Name: s.Name, RefType: "envFrom"})
				}
			}
			for _, env := range container.Env {
				if env.ValueFrom != nil {
					if env.ValueFrom.ConfigMapKeyRef != nil && kind == "ConfigMap" && env.ValueFrom.ConfigMapKeyRef.Name == name {
						refs = append(refs, WorkloadRef{Kind: "StatefulSet", Name: s.Name, RefType: "env"})
					}
					if env.ValueFrom.SecretKeyRef != nil && kind == "Secret" && env.ValueFrom.SecretKeyRef.Name == name {
						refs = append(refs, WorkloadRef{Kind: "StatefulSet", Name: s.Name, RefType: "env"})
					}
				}
			}
		}
		for _, vol := range s.Spec.Template.Spec.Volumes {
			if vol.ConfigMap != nil && kind == "ConfigMap" && vol.ConfigMap.Name == name {
				refs = append(refs, WorkloadRef{Kind: "StatefulSet", Name: s.Name, RefType: "volume"})
			}
			if vol.Secret != nil && kind == "Secret" && vol.Secret.SecretName == name {
				refs = append(refs, WorkloadRef{Kind: "StatefulSet", Name: s.Name, RefType: "volume"})
			}
		}
	}
	return refs
}
func findRefsInDaemonSets(items []appsv1.DaemonSet, name, kind string) []WorkloadRef {
	var refs []WorkloadRef
	for _, d := range items {
		for _, container := range d.Spec.Template.Spec.Containers {
			for _, envFrom := range container.EnvFrom {
				if envFrom.ConfigMapRef != nil && kind == "ConfigMap" && envFrom.ConfigMapRef.Name == name {
					refs = append(refs, WorkloadRef{Kind: "DaemonSet", Name: d.Name, RefType: "envFrom"})
				}
				if envFrom.SecretRef != nil && kind == "Secret" && envFrom.SecretRef.Name == name {
					refs = append(refs, WorkloadRef{Kind: "DaemonSet", Name: d.Name, RefType: "envFrom"})
				}
			}
			for _, env := range container.Env {
				if env.ValueFrom != nil {
					if env.ValueFrom.ConfigMapKeyRef != nil && kind == "ConfigMap" && env.ValueFrom.ConfigMapKeyRef.Name == name {
						refs = append(refs, WorkloadRef{Kind: "DaemonSet", Name: d.Name, RefType: "env"})
					}
					if env.ValueFrom.SecretKeyRef != nil && kind == "Secret" && env.ValueFrom.SecretKeyRef.Name == name {
						refs = append(refs, WorkloadRef{Kind: "DaemonSet", Name: d.Name, RefType: "env"})
					}
				}
			}
		}
		for _, vol := range d.Spec.Template.Spec.Volumes {
			if vol.ConfigMap != nil && kind == "ConfigMap" && vol.ConfigMap.Name == name {
				refs = append(refs, WorkloadRef{Kind: "DaemonSet", Name: d.Name, RefType: "volume"})
			}
			if vol.Secret != nil && kind == "Secret" && vol.Secret.SecretName == name {
				refs = append(refs, WorkloadRef{Kind: "DaemonSet", Name: d.Name, RefType: "volume"})
			}
		}
	}
	return refs
}
