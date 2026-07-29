package k8s

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpsertAliDNSCredentialSecret writes the only Secret shape accepted by the managed AliDNS solver.
func (c *Client) UpsertAliDNSCredentialSecret(namespace, name, accessKeyID, accessKeySecret string) error {
	if c == nil || c.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	secrets := c.Clientset.CoreV1().Secrets(namespace)
	current, err := secrets.Get(c.Ctx(), name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = secrets.Create(c.Ctx(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{"app.kubernetes.io/managed-by": "cylism-manager", "cylism.io/dns-provider": "alidns"}}, Type: corev1.SecretTypeOpaque, StringData: map[string]string{"access-key-id": accessKeyID, "access-key-secret": accessKeySecret}}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	current.StringData = map[string]string{"access-key-id": accessKeyID, "access-key-secret": accessKeySecret}
	if current.Labels == nil {
		current.Labels = map[string]string{}
	}
	current.Labels["app.kubernetes.io/managed-by"] = "cylism-manager"
	current.Labels["cylism.io/dns-provider"] = "alidns"
	_, err = secrets.Update(c.Ctx(), current, metav1.UpdateOptions{})
	return err
}

func (c *Client) DeleteAliDNSCredentialSecret(namespace, name string) error {
	if c == nil || c.Clientset == nil {
		return fmt.Errorf("Kubernetes 客户端未初始化")
	}
	err := c.Clientset.CoreV1().Secrets(namespace).Delete(c.Ctx(), name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}
