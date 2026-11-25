package k8s

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func (c *Client) PrepareApplicationNamespaceCertificates(ctx context.Context, move ApplicationNamespaceMove, marker string, certificates map[string]string, action string) error {
	if len(certificates) == 0 {
		return nil
	}
	client, err := c.dynamicClient()
	if err != nil {
		return err
	}
	source := client.Resource(applicationCertificateStatusGVR).Namespace(move.SourceNamespace)
	target := client.Resource(applicationCertificateStatusGVR).Namespace(move.TargetNamespace)
	for name, secret := range certificates {
		if action == "cancel" {
			obj, err := target.Get(ctx, name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return err
			}
			if obj.GetAnnotations()[applicationMoveAnnotation] == marker {
				uid := obj.GetUID()
				if err := target.Delete(ctx, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil && !apierrors.IsNotFound(err) {
					return err
				}
			}
			continue
		}
		original, err := source.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("读取源证书 %s 失败: %w", name, err)
		}
		spec, _, err := unstructured.NestedMap(original.Object, "spec")
		if err != nil || spec["secretName"] != secret {
			return fmt.Errorf("源证书 %s 的 Secret 与域名配置不一致", name)
		}
		issuer, _, _ := unstructured.NestedString(spec, "issuerRef", "kind")
		if issuer != "ClusterIssuer" {
			return fmt.Errorf("证书 %s 需要使用 ClusterIssuer", name)
		}
		obj, err := target.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) && action == "check" {
			continue
		}
		if apierrors.IsNotFound(err) && action == "prepare" {
			obj = &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "cert-manager.io/v1", "kind": "Certificate", "spec": spec}}
			obj.SetName(name)
			obj.SetNamespace(move.TargetNamespace)
			obj.SetLabels(map[string]string{ManagedByLabel: ManagedByValue, EnvironmentLabel: EnvironmentLabelValue(move.TargetEnvironmentID)})
			obj.SetAnnotations(map[string]string{applicationMoveAnnotation: marker})
			obj, err = target.Create(ctx, obj, metav1.CreateOptions{})
		}
		if err != nil {
			return fmt.Errorf("目标证书 %s 不可用: %w", name, err)
		}
		if marker == "" || checkNamespaceMoveResource(obj, nil, marker) != nil || namespaceMoveHash(obj.Object["spec"]) != namespaceMoveHash(spec) {
			return fmt.Errorf("目标证书 %s 已变更或同名占用", name)
		}
		if action == "verify" {
			if info := certToInfo(obj); info.Status != "Ready" {
				return fmt.Errorf("目标证书 %s 尚未就绪，源环境可继续运行", name)
			}
		}
	}
	return nil
}
