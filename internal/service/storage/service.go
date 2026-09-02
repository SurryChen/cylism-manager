package storage

import (
	"context"
	"errors"
	"path"
	"strings"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	corev1 "k8s.io/api/core/v1"
)

// ValidateHostDirectoryImportPath normalizes an import source and rejects
// operating-system and K3s data directories.
func ValidateHostDirectoryImportPath(value string) (string, bool) {
	cleaned := path.Clean(strings.TrimSpace(value))
	if !strings.HasPrefix(cleaned, "/") || cleaned == "/" {
		return "", false
	}
	for _, protected := range []string{"/boot", "/dev", "/etc", "/proc", "/run", "/sys", "/usr", "/bin", "/sbin", "/lib", "/lib64", "/var/lib/rancher/k3s", "/var/lib/kubelet"} {
		if cleaned == protected || strings.HasPrefix(cleaned, protected+"/") {
			return "", false
		}
	}
	return cleaned, true
}

func HostDirectoryImportPathsOverlap(left, right string) bool {
	left, right = path.Clean(left), path.Clean(right)
	return left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func ValidateBackupRoot(value string) (string, error) {
	root := path.Clean(strings.TrimSpace(value))
	if !strings.HasPrefix(root, "/") || root == "/" {
		return "", errors.New("备份根目录必须是非根目录的绝对路径")
	}
	return root, nil
}

// PVCRepositoryAdapter is the PVC resource subset used by the storage service.
// Keeping this interface narrow makes PVC workflows testable without a live cluster.
type PVCRepositoryAdapter interface {
	ListPVCsContext(context.Context, string) ([]k8sclient.PersistentVolumeClaimInfo, error)
	ListManagedPVCsContext(context.Context, string, uint) ([]k8sclient.PersistentVolumeClaimInfo, error)
	GetManagedPVCContext(context.Context, string, string, uint) (*k8sclient.PersistentVolumeClaimInfo, error)
	CreateManagedPVCContext(context.Context, string, uint, k8sclient.PersistentVolumeClaimRequest) (*corev1.PersistentVolumeClaim, error)
	DeleteManagedPVCContext(context.Context, string, string, uint) error
	ListStorageClassesContext(context.Context) ([]k8sclient.StorageClassInfo, error)
}

// EnvironmentStore is the persistence subset required for namespace resolution.
type EnvironmentStore interface {
	GetEnvironmentByID(id uint) (*model.Environment, error)
}

// RecordStore is the durable task state required by asynchronous PVC
// workflows. It stays distinct from environment lookup so callers can supply
// a read-only environment source where task records are unavailable.
type RecordStore interface {
	ListPersistentVolumeMigrations(uint) ([]model.PersistentVolumeMigration, error)
	GetPersistentVolumeMigration(uint) (*model.PersistentVolumeMigration, error)
	FindActivePVCMigration(uint, string) (*model.PersistentVolumeMigration, error)
	CreatePersistentVolumeMigration(*model.PersistentVolumeMigration) error
	UpdatePersistentVolumeMigration(*model.PersistentVolumeMigration, string, string) error
	ListPersistentVolumeBackups(uint, string) ([]model.PersistentVolumeBackup, error)
	GetPersistentVolumeBackup(uint) (*model.PersistentVolumeBackup, error)
	CreatePersistentVolumeBackup(*model.PersistentVolumeBackup) error
	UpdatePersistentVolumeBackup(*model.PersistentVolumeBackup) error
	ListHostDirectoryPVCImports(uint, string) ([]model.HostDirectoryPVCImport, error)
	GetHostDirectoryPVCImport(uint) (*model.HostDirectoryPVCImport, error)
	FindActiveHostDirectoryPVCImport(uint, string) (*model.HostDirectoryPVCImport, error)
	CreateHostDirectoryPVCImport(*model.HostDirectoryPVCImport) error
	UpdateHostDirectoryPVCImport(*model.HostDirectoryPVCImport, string, string) error
}

type Service struct {
	k8s          PVCRepositoryAdapter
	environments EnvironmentStore
	records      RecordStore
	executor     AsyncExecutor
}

// AsyncExecutor contains the infrastructure-specific execution hooks for
// long-running storage operations. The service owns dispatch and lifecycle
// boundaries while the API package supplies the SSH/Kubernetes implementation
// during the compatibility migration.
type AsyncExecutor struct {
	RunMigration func(context.Context, uint, string)
	RunImport    func(context.Context, uint, bool)
	RunBackup    func(context.Context, uint)
	RunRestore   func(context.Context, uint)
}

// ValidatePVCDeletion centralizes the destructive-operation policy shared by
// HTTP, CLI and background callers. Kubernetes deletion itself remains in the
// adapter; this method only evaluates durable business constraints.
func (s *Service) ValidatePVCDeletion(confirmDataDelete, migrationActive bool, references []string, reclaimPolicy string) error {
	if migrationActive {
		return errors.New("存储卷正在迁移，不能删除")
	}
	if len(references) > 0 {
		return errors.New("PVC 仍被应用模板或工作负载引用，不能删除")
	}
	if !confirmDataDelete {
		policy := strings.TrimSpace(reclaimPolicy)
		if policy == "" {
			policy = "未知"
		}
		return errors.New("删除 PVC 可能影响底层数据（PV 回收策略：" + policy + "），请确认后重试")
	}
	return nil
}

func NewService(k8s PVCRepositoryAdapter, environments EnvironmentStore, records ...RecordStore) *Service {
	service := &Service{k8s: k8s, environments: environments}
	if len(records) > 0 {
		service.records = records[0]
	}
	return service
}

// WithAsyncExecutor installs the execution adapter used by background tasks.
// It returns the service to support construction-time composition.
func (s *Service) WithAsyncExecutor(executor AsyncExecutor) *Service {
	s.executor = executor
	return s
}

func (s *Service) StartMigration(ctx context.Context, id uint, helperImage string) error {
	if s.executor.RunMigration == nil {
		return errors.New("存储卷迁移执行器未初始化")
	}
	go s.executor.RunMigration(context.WithoutCancel(ctx), id, helperImage)
	return nil
}

func (s *Service) StartImport(ctx context.Context, id uint, replaceTarget bool) error {
	if s.executor.RunImport == nil {
		return errors.New("目录导入执行器未初始化")
	}
	go s.executor.RunImport(context.WithoutCancel(ctx), id, replaceTarget)
	return nil
}

func (s *Service) StartBackup(ctx context.Context, id uint) error {
	if s.executor.RunBackup == nil {
		return errors.New("存储卷备份执行器未初始化")
	}
	go s.executor.RunBackup(context.WithoutCancel(ctx), id)
	return nil
}

func (s *Service) StartRestore(ctx context.Context, id uint) error {
	if s.executor.RunRestore == nil {
		return errors.New("存储卷恢复执行器未初始化")
	}
	go s.executor.RunRestore(context.WithoutCancel(ctx), id)
	return nil
}

func (s *Service) requireRecords() (RecordStore, error) {
	if s.records == nil {
		return nil, errors.New("数据存储未初始化")
	}
	return s.records, nil
}

func (s *Service) ListMigrations(environmentID uint) ([]model.PersistentVolumeMigration, error) {
	records, err := s.requireRecords()
	if err != nil {
		return nil, err
	}
	return records.ListPersistentVolumeMigrations(environmentID)
}
func (s *Service) GetMigration(id uint) (*model.PersistentVolumeMigration, error) {
	records, err := s.requireRecords()
	if err != nil {
		return nil, err
	}
	return records.GetPersistentVolumeMigration(id)
}
func (s *Service) FindActiveMigration(environmentID uint, pvc string) (*model.PersistentVolumeMigration, error) {
	records, err := s.requireRecords()
	if err != nil {
		return nil, err
	}
	return records.FindActivePVCMigration(environmentID, pvc)
}
func (s *Service) CreateMigration(migration *model.PersistentVolumeMigration) error {
	records, err := s.requireRecords()
	if err != nil {
		return err
	}
	return records.CreatePersistentVolumeMigration(migration)
}
func (s *Service) UpdateMigration(migration *model.PersistentVolumeMigration, status, detail string) error {
	records, err := s.requireRecords()
	if err != nil {
		return err
	}
	return records.UpdatePersistentVolumeMigration(migration, status, detail)
}
func (s *Service) ListBackups(environmentID uint, pvc string) ([]model.PersistentVolumeBackup, error) {
	records, err := s.requireRecords()
	if err != nil {
		return nil, err
	}
	return records.ListPersistentVolumeBackups(environmentID, pvc)
}
func (s *Service) GetBackup(id uint) (*model.PersistentVolumeBackup, error) {
	records, err := s.requireRecords()
	if err != nil {
		return nil, err
	}
	return records.GetPersistentVolumeBackup(id)
}
func (s *Service) CreateBackup(backup *model.PersistentVolumeBackup) error {
	records, err := s.requireRecords()
	if err != nil {
		return err
	}
	return records.CreatePersistentVolumeBackup(backup)
}
func (s *Service) UpdateBackup(backup *model.PersistentVolumeBackup) error {
	records, err := s.requireRecords()
	if err != nil {
		return err
	}
	return records.UpdatePersistentVolumeBackup(backup)
}
func (s *Service) ListImports(environmentID uint, pvc string) ([]model.HostDirectoryPVCImport, error) {
	records, err := s.requireRecords()
	if err != nil {
		return nil, err
	}
	return records.ListHostDirectoryPVCImports(environmentID, pvc)
}
func (s *Service) GetImport(id uint) (*model.HostDirectoryPVCImport, error) {
	records, err := s.requireRecords()
	if err != nil {
		return nil, err
	}
	return records.GetHostDirectoryPVCImport(id)
}
func (s *Service) FindActiveImport(environmentID uint, pvc string) (*model.HostDirectoryPVCImport, error) {
	records, err := s.requireRecords()
	if err != nil {
		return nil, err
	}
	return records.FindActiveHostDirectoryPVCImport(environmentID, pvc)
}
func (s *Service) CreateImport(task *model.HostDirectoryPVCImport) error {
	records, err := s.requireRecords()
	if err != nil {
		return err
	}
	return records.CreateHostDirectoryPVCImport(task)
}
func (s *Service) UpdateImport(task *model.HostDirectoryPVCImport, status, detail string) error {
	records, err := s.requireRecords()
	if err != nil {
		return err
	}
	return records.UpdateHostDirectoryPVCImport(task, status, detail)
}

func (s *Service) ListPVCsContext(ctx context.Context, namespace string, environmentID uint) ([]k8sclient.PersistentVolumeClaimInfo, error) {
	if s.k8s == nil {
		return nil, errors.New("Kubernetes 存储适配器未初始化")
	}
	if environmentID != 0 {
		env, err := s.environments.GetEnvironmentByID(environmentID)
		if err != nil {
			return nil, errors.New("环境不存在")
		}
		if namespace != "" && strings.TrimSpace(namespace) != env.Namespace {
			return nil, errors.New("命名空间与环境不匹配")
		}
		return s.k8s.ListManagedPVCsContext(ctx, env.Namespace, environmentID)
	}
	return s.k8s.ListPVCsContext(ctx, strings.TrimSpace(namespace))
}

// GetPVC resolves and reads one claim through the storage adapter. Keeping
// this lookup beside ListPVCs prevents HTTP adapters from reaching into K8s.
func (s *Service) GetPVCContext(ctx context.Context, namespace, name string, environmentID uint) (*k8sclient.PersistentVolumeClaimInfo, error) {
	if s.k8s == nil {
		return nil, errors.New("Kubernetes 存储适配器未初始化")
	}
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("PVC 名称必填")
	}
	resolved, err := s.ResolveNamespace(namespace, environmentID)
	if err != nil {
		return nil, err
	}
	return s.k8s.GetManagedPVCContext(ctx, resolved, strings.TrimSpace(name), environmentID)
}

func (s *Service) ListStorageClassesContext(ctx context.Context) ([]k8sclient.StorageClassInfo, error) {
	if s.k8s == nil {
		return nil, errors.New("Kubernetes 存储适配器未初始化")
	}
	return s.k8s.ListStorageClassesContext(ctx)
}

func (s *Service) CreatePVCContext(ctx context.Context, namespace string, environmentID uint, request k8sclient.PersistentVolumeClaimRequest) (*corev1.PersistentVolumeClaim, error) {
	if s.k8s == nil {
		return nil, errors.New("Kubernetes 存储适配器未初始化")
	}
	resolved, err := s.ResolveNamespace(namespace, environmentID)
	if err != nil {
		return nil, err
	}
	return s.k8s.CreateManagedPVCContext(ctx, resolved, environmentID, request)
}

func (s *Service) ResolveNamespace(namespace string, environmentID uint) (string, error) {
	namespace = strings.TrimSpace(namespace)
	if environmentID == 0 {
		if namespace == "" {
			return "", errors.New("命名空间必填")
		}
		return namespace, nil
	}
	if s.environments == nil {
		return "", errors.New("数据存储未初始化")
	}
	env, err := s.environments.GetEnvironmentByID(environmentID)
	if err != nil {
		return "", errors.New("环境不存在")
	}
	if namespace != "" && namespace != env.Namespace {
		return "", errors.New("命名空间与环境不匹配")
	}
	return env.Namespace, nil
}

func (s *Service) DeletePVCContext(ctx context.Context, namespace, name string, environmentID uint) error {
	if s.k8s == nil {
		return errors.New("Kubernetes 存储适配器未初始化")
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("PVC 名称必填")
	}
	return s.k8s.DeleteManagedPVCContext(ctx, namespace, strings.TrimSpace(name), environmentID)
}
