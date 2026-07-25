package k8s

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkloadRef 工作负载引用
type WorkloadRef struct {
	Kind    string `json:"kind"`     // Deployment / StatefulSet / DaemonSet
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

// ListConfigMaps 列出所有 ConfigMap
func (c *Client) ListConfigMaps(ns string) ([]ConfigMapInfo, error) {
	var list *corev1.ConfigMapList
	var err error
	if ns == "" {
		list, err = c.Clientset.CoreV1().ConfigMaps("").List(c.ctx, metav1.ListOptions{})
	} else {
		list, err = c.Clientset.CoreV1().ConfigMaps(ns).List(c.ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("list configmaps: %w", err)
	}

	result := make([]ConfigMapInfo, 0, len(list.Items))
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
			UsedBy:    c.findConfigRefs(cm.Namespace, cm.Name, "ConfigMap"),
			Age:       timeAgo(cm.CreationTimestamp.Time),
		})
	}
	return result, nil
}

// GetConfigMap 获取 ConfigMap 详情
func (c *Client) GetConfigMap(namespace, name string) (*ConfigMapDetail, error) {
	cm, err := c.Clientset.CoreV1().ConfigMaps(namespace).Get(c.ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get configmap %s/%s: %w", namespace, name, err)
	}
	return &ConfigMapDetail{
		Name:      cm.Name,
		Namespace: cm.Namespace,
		Data:      cm.Data,
		UsedBy:    c.findConfigRefs(namespace, name, "ConfigMap"),
		Age:       timeAgo(cm.CreationTimestamp.Time),
	}, nil
}

// ListSecrets 列出所有 Secret（不返回 value）
func (c *Client) ListSecrets(ns string) ([]SecretInfo, error) {
	var list *corev1.SecretList
	var err error
	if ns == "" {
		list, err = c.Clientset.CoreV1().Secrets("").List(c.ctx, metav1.ListOptions{})
	} else {
		list, err = c.Clientset.CoreV1().Secrets(ns).List(c.ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}

	result := make([]SecretInfo, 0, len(list.Items))
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
			UsedBy:    c.findConfigRefs(s.Namespace, s.Name, "Secret"),
			Age:       timeAgo(s.CreationTimestamp.Time),
		})
	}
	return result, nil
}

// GetSecret 获取 Secret 详情（含 base64 value）
func (c *Client) GetSecret(namespace, name string) (*SecretDetail, error) {
	s, err := c.Clientset.CoreV1().Secrets(namespace).Get(c.ctx, name, metav1.GetOptions{})
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
		UsedBy:    c.findConfigRefs(namespace, name, "Secret"),
		Age:       timeAgo(s.CreationTimestamp.Time),
	}, nil
}

// findConfigRefs 查找引用指定 ConfigMap/Secret 的工作负载
func (c *Client) findConfigRefs(ns, name string, kind string) []WorkloadRef {
	var refs []WorkloadRef

	// 查 Deployment
	deploys, err := c.Clientset.AppsV1().Deployments(ns).List(c.ctx, metav1.ListOptions{})
	if err == nil {
		refs = append(refs, findRefsInDeployments(deploys.Items, name, kind)...)
	}

	// 查 StatefulSet
	sts, err := c.Clientset.AppsV1().StatefulSets(ns).List(c.ctx, metav1.ListOptions{})
	if err == nil {
		refs = append(refs, findRefsInStatefulSets(sts.Items, name, kind)...)
	}

	// 查 DaemonSet
	ds, err := c.Clientset.AppsV1().DaemonSets(ns).List(c.ctx, metav1.ListOptions{})
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
