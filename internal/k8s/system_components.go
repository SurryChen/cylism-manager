package k8s

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var helmChartConfigGVR = schema.GroupVersionResource{
	Group:    "helm.cattle.io",
	Version:  "v1",
	Resource: "helmchartconfigs",
}

// GetHelmChartConfig 返回 HelmChartConfig CRD，不存在时返回 nil。
func (c *Client) GetHelmChartConfig(ctx context.Context, namespace, name string) (*unstructured.Unstructured, error) {
	if c == nil || c.DynamicClient == nil {
		return nil, fmt.Errorf("Kubernetes 动态客户端未初始化")
	}
	obj, err := c.DynamicClient.Resource(helmChartConfigGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 HelmChartConfig %s/%s: %w", namespace, name, err)
	}
	return obj, nil
}

// ApplyHelmChartConfig 创建或更新 HelmChartConfig CRD；K3s helm-controller
// 会按 CRD 重新渲染内置 chart，因此配置在升级后依然保留。
func (c *Client) ApplyHelmChartConfig(ctx context.Context, namespace, name, valuesContent string) error {
	if c == nil || c.DynamicClient == nil {
		return fmt.Errorf("Kubernetes 动态客户端未初始化")
	}
	desired := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "helm.cattle.io/v1",
		"kind":       "HelmChartConfig",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels":    map[string]interface{}{ManagedByLabel: ManagedByValue},
		},
		"spec": map[string]any{"valuesContent": valuesContent},
	}}
	current, err := c.GetHelmChartConfig(ctx, namespace, name)
	if err != nil {
		return err
	}
	resource := c.DynamicClient.Resource(helmChartConfigGVR).Namespace(namespace)
	if current == nil {
		if _, err := resource.Create(ctx, desired, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("创建 HelmChartConfig %s/%s: %w", namespace, name, err)
		}
		return nil
	}
	desired.SetResourceVersion(current.GetResourceVersion())
	if _, err := resource.Update(ctx, desired, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("更新 HelmChartConfig %s/%s: %w", namespace, name, err)
	}
	return nil
}

// DeleteHelmChartConfig 删除 CRD，K3s 恢复内置 chart 默认值。
func (c *Client) DeleteHelmChartConfig(ctx context.Context, namespace, name string) error {
	if c == nil || c.DynamicClient == nil {
		return fmt.Errorf("Kubernetes 动态客户端未初始化")
	}
	err := c.DynamicClient.Resource(helmChartConfigGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("删除 HelmChartConfig %s/%s: %w", namespace, name, err)
	}
	return nil
}
