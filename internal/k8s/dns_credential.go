package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpsertDNSCredentialSecret writes one controlled provider Secret. Values must already pass adapter validation.
func (c *Client) UpsertDNSCredentialSecretContext(ctx context.Context, providerID, namespace, name string, values map[string]string) error {
	if c == nil || c.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	provider, ok := GetDNSProvider(providerID)
	if !ok {
		return fmt.Errorf("不支持的 DNS Provider: %s", providerID)
	}
	if err := provider.ValidateValues(values); err != nil {
		return err
	}
	data := provider.SecretData(values)
	secrets := c.Clientset.CoreV1().Secrets(namespace)
	current, err := secrets.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = secrets.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{"app.kubernetes.io/managed-by": "cylism-manager", "cylism.io/dns-provider": providerID}}, Type: corev1.SecretTypeOpaque, StringData: data}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	current.StringData = data
	if current.Labels == nil {
		current.Labels = map[string]string{}
	}
	current.Labels["app.kubernetes.io/managed-by"] = "cylism-manager"
	current.Labels["cylism.io/dns-provider"] = providerID
	_, err = secrets.Update(ctx, current, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteDNSCredentialSecretContext(ctx context.Context, namespace, name string) error {
	if c == nil || c.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	err := c.Clientset.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}
