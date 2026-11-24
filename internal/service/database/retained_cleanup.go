package database

import (
	"context"
	"fmt"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Names alone are insufficient for Secret ownership: applications in the same
// namespace may use similarly named credentials. Root Secrets are created by
// the platform; the other Operator Secrets must reference this exact cluster.
func clusterOwnsSecret(secret corev1.Secret, item *model.MySQLCluster) bool {
	if secret.Name == item.ResourceName+"-root" {
		return secret.Labels[k8s.ManagedByLabel] == k8s.ManagedByValue
	}
	for _, ref := range secret.OwnerReferences {
		if ref.Kind == "InnoDBCluster" && ref.APIVersion == "mysql.oracle.com/v2" && ref.Name == item.ResourceName {
			return true
		}
	}
	return false
}

// CleanupRetainedClusterResources only runs after the CR, every workload and
// every Pod have disappeared. Completion means the selected objects are gone,
// not merely that Kubernetes accepted delete requests. Repeated calls are safe.
func CleanupRetainedClusterResources(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, deleteClaims, deleteSecrets bool) (bool, string, error) {
	state, err := InspectClusterCleanup(ctx, client, item)
	if err != nil {
		return false, "", err
	}
	if state.ClusterResource || len(state.Workloads) > 0 || len(state.Pods) > 0 {
		return false, "", fmt.Errorf("集群资源或负载尚未完全退出，不能清理数据卷和密钥")
	}
	if deleteClaims {
		for _, name := range state.Claims {
			claims := client.Clientset.CoreV1().PersistentVolumeClaims(item.Namespace)
			claim, err := claims.Get(ctx, name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return false, "", err
			}
			uid := claim.UID
			if err := claims.Delete(ctx, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil && !apierrors.IsNotFound(err) {
				return false, "", fmt.Errorf("清理数据卷 %s：%w", name, err)
			}
		}
		state, err = InspectClusterCleanup(ctx, client, item)
		if err != nil {
			return false, "", err
		}
		if len(state.Claims) > 0 {
			return false, fmt.Sprintf("等待 %d 个数据卷删除完成，密钥暂时保留", len(state.Claims)), nil
		}
	}
	if deleteSecrets {
		for _, name := range state.Secrets {
			secrets := client.Clientset.CoreV1().Secrets(item.Namespace)
			secret, err := secrets.Get(ctx, name, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return false, "", err
			}
			if !clusterOwnsSecret(*secret, item) {
				return false, "", fmt.Errorf("密钥 %s 归属已变化，停止清理", name)
			}
			uid := secret.UID
			if err := secrets.Delete(ctx, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil && !apierrors.IsNotFound(err) {
				return false, "", fmt.Errorf("清理密钥 %s：%w", name, err)
			}
		}
	}
	state, err = InspectClusterCleanup(ctx, client, item)
	if err != nil {
		return false, "", err
	}
	if deleteSecrets && len(state.Secrets) > 0 {
		return false, fmt.Sprintf("等待 %d 个密钥删除完成", len(state.Secrets)), nil
	}
	claims, secrets := "保留数据卷", "保留密钥"
	if deleteClaims {
		claims = "数据卷已清理"
	}
	if deleteSecrets {
		secrets = "密钥已清理"
	}
	return true, fmt.Sprintf("集群资源已清退；%s；%s。可删除平台记录", claims, secrets), nil
}
