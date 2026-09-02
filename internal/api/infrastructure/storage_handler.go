package infrastructure

import (
	"context"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/repository"
	"github.com/cylism/cylism-manager/internal/service/storage"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StorageHandler is the infrastructure HTTP boundary for PVC endpoints. The
// underlying workflows are injected so long-running migration/import jobs can
// retain their existing recovery behaviour while they are progressively moved
// into the storage service.
type StorageHandler struct {
	store     repository.PVCRepository
	pvc       PVCRepositoryAdapter
	migration PVCMigrationReconciler
	workloads PVCWorkloadReader
	encKey    []byte
	Service   *storage.Service
}

// PVCRepositoryAdapter owns PVC CRUD and StorageClass queries. It is also the
// only Kubernetes capability required by the storage Service itself.
type PVCRepositoryAdapter = storage.PVCRepositoryAdapter

// PVCMigrationReconciler owns only the resource changes and wait operations
// required by migration, import and backup workflows.
type PVCMigrationReconciler interface {
	GetNodeInfo(context.Context, string) (*k8sclient.NodeInfo, error)
	CreatePVCBindingPod(context.Context, string, string, string, string, string) (*corev1.Pod, error)
	DeletePVCBindingPod(context.Context, string, string) error
	WaitForManagedPVCBound(context.Context, string, string, uint) (*k8sclient.PersistentVolumeClaimInfo, error)
	ReplaceDeploymentPVCNode(context.Context, string, string, string, string, string) (int32, error)
	DeploymentUsingPVC(context.Context, string, string) ([]appsv1.Deployment, error)
	ScaleDeployment(context.Context, string, string, int32) error
	ListDeploymentPods(context.Context, string, string) ([]k8sclient.PodRef, error)
}

// PVCWorkloadReader provides non-mutating namespace and workload inspection
// used for validation and reference reporting.
type PVCWorkloadReader interface {
	NamespaceExists(context.Context, string) error
	ListDeployments(context.Context, string) ([]appsv1.Deployment, error)
	ListStatefulSets(context.Context, string) ([]appsv1.StatefulSet, error)
	GetDeployment(context.Context, string, string) (*appsv1.Deployment, error)
}

type clientPVCReconciler struct{ client *k8sclient.Client }

func newClientPVCReconciler(client *k8sclient.Client) *clientPVCReconciler {
	if client == nil {
		return nil
	}
	return &clientPVCReconciler{client: client}
}
func (r clientPVCReconciler) ListPVCs(ns string) ([]k8sclient.PersistentVolumeClaimInfo, error) {
	return r.client.ListPVCs(ns)
}
func (r clientPVCReconciler) ListManagedPVCs(ns string, id uint) ([]k8sclient.PersistentVolumeClaimInfo, error) {
	return r.client.ListManagedPVCs(ns, id)
}
func (r clientPVCReconciler) GetManagedPVC(ns, name string, id uint) (*k8sclient.PersistentVolumeClaimInfo, error) {
	return r.client.GetManagedPVC(ns, name, id)
}
func (r clientPVCReconciler) CreateManagedPVC(ns string, id uint, req k8sclient.PersistentVolumeClaimRequest) (*corev1.PersistentVolumeClaim, error) {
	return r.client.CreateManagedPVC(ns, id, req)
}
func (r clientPVCReconciler) DeleteManagedPVC(ns, name string, id uint) error {
	return r.client.DeleteManagedPVC(ns, name, id)
}
func (r clientPVCReconciler) ListStorageClasses() ([]k8sclient.StorageClassInfo, error) {
	return r.client.ListStorageClasses()
}
func (r clientPVCReconciler) GetNodeInfo(ctx context.Context, name string) (*k8sclient.NodeInfo, error) {
	return r.client.GetNodeInfoContext(ctx, name)
}
func (r clientPVCReconciler) CreatePVCBindingPod(ctx context.Context, ns, migration, claim, node, image string) (*corev1.Pod, error) {
	return r.client.CreatePVCBindingPodContext(ctx, ns, migration, claim, node, image)
}
func (r clientPVCReconciler) DeletePVCBindingPod(ctx context.Context, ns, migration string) error {
	return r.client.DeletePVCBindingPodContext(ctx, ns, migration)
}
func (r clientPVCReconciler) WaitForManagedPVCBound(ctx context.Context, ns, name string, id uint) (*k8sclient.PersistentVolumeClaimInfo, error) {
	return r.client.WaitForManagedPVCBound(ctx, ns, name, id)
}
func (r clientPVCReconciler) ReplaceDeploymentPVCNode(ctx context.Context, ns, name, source, target, node string) (int32, error) {
	return r.client.ReplaceDeploymentPVCNodeContext(ctx, ns, name, source, target, node)
}
func (r clientPVCReconciler) DeploymentUsingPVC(ctx context.Context, ns, claim string) ([]appsv1.Deployment, error) {
	return r.client.DeploymentUsingPVCContext(ctx, ns, claim)
}
func (r clientPVCReconciler) ScaleDeployment(ctx context.Context, ns, name string, replicas int32) error {
	return r.client.ScaleDeploymentContext(ctx, ns, name, replicas)
}
func (r clientPVCReconciler) ListDeploymentPods(ctx context.Context, ns, name string) ([]k8sclient.PodRef, error) {
	return r.client.ListDeploymentPodsContext(ctx, ns, name)
}
func (r clientPVCReconciler) NamespaceExists(ctx context.Context, name string) error {
	_, err := r.client.Clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	return err
}
func (r clientPVCReconciler) ListDeployments(ctx context.Context, namespace string) ([]appsv1.Deployment, error) {
	list, err := r.client.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}
func (r clientPVCReconciler) ListStatefulSets(ctx context.Context, namespace string) ([]appsv1.StatefulSet, error) {
	list, err := r.client.Clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}
func (r clientPVCReconciler) GetDeployment(ctx context.Context, namespace, name string) (*appsv1.Deployment, error) {
	return r.client.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
}
func NewStorageHandlerWithClient(service *storage.Service, st repository.PVCRepository, encKey []byte, client *k8sclient.Client) *StorageHandler {
	adapter := newClientPVCReconciler(client)
	return &StorageHandler{Service: service, store: st, encKey: encKey, pvc: adapter, migration: adapter, workloads: adapter}
}

func (h *StorageHandler) ConfigureStorageExecutor() {
	if h.Service == nil {
		return
	}
	h.Service.WithAsyncExecutor(storage.AsyncExecutor{
		RunMigration: h.runPersistentVolumeMigration,
		RunImport:    h.runHostDirectoryPVCImport,
		RunBackup:    h.executePersistentVolumeBackup,
		RunRestore:   h.executePersistentVolumeRestore,
	})
}

func storageK8sUnavailable(c *gin.Context) {
	apiShared.K8sUnavailable(c)
}

func storageRequestUserID(c *gin.Context) uint {
	return apiShared.UserID(c)
}

func storageErrorsIsNotFound(err error) bool { return err == gorm.ErrRecordNotFound }
