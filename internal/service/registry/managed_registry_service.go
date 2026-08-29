package registry

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	"gorm.io/gorm"
)

// ManagedRegistryService owns the persistent lifecycle of a platform-managed
// OCI Registry. Kubernetes reconciliation is intentionally supplied by the
// infrastructure adapter at the use-case boundary.
type ManagedRegistryService struct {
	repository repository.ManagedRegistryRepository
	encKey     []byte
}

func NewManagedRegistryService(repository repository.ManagedRegistryRepository, encKey []byte) *ManagedRegistryService {
	return &ManagedRegistryService{repository: repository, encKey: encKey}
}

func (s *ManagedRegistryService) List() ([]model.ManagedOCIRegistry, error) {
	return s.repository.ListManagedOCIRegistries()
}

func (s *ManagedRegistryService) Get(id uint) (*model.ManagedOCIRegistry, error) {
	return s.repository.GetManagedOCIRegistry(id)
}

func (s *ManagedRegistryService) EnsureEndpointAvailable(endpoint string) error {
	if registry, err := s.repository.GetImageRegistryByEndpoint(endpoint); err == nil {
		if registry.ManagedRegistryID == nil {
			return fmt.Errorf("镜像仓库地址已由非受管记录 %q 使用", registry.Name)
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("读取镜像仓库地址占用失败: %w", err)
	}
	if mirror, err := s.repository.GetNodeRegistryMirrorByRegistry(endpoint); err == nil {
		if mirror.ManagedRegistryID == nil {
			return fmt.Errorf("节点镜像源地址已由非受管记录 %q 使用", mirror.Name)
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("读取节点镜像源地址占用失败: %w", err)
	}
	return nil
}

func (s *ManagedRegistryService) PersistCreate(registry *model.ManagedOCIRegistry, password string, createdBy uint, projectIDs []uint) error {
	registries, err := s.repository.ListManagedOCIRegistries()
	if err != nil {
		return fmt.Errorf("读取现有制品库失败: %w", err)
	}
	if len(registries) > 0 {
		return errors.New("首期仅支持一个受管制品库")
	}
	if err := s.EnsureEndpointAvailable(registry.Endpoint); err != nil {
		return err
	}
	credential, err := EncryptCredential(s.encKey, password)
	if err != nil {
		return fmt.Errorf("加密制品库凭据失败: %w", err)
	}
	registry.EncryptedCredential, registry.CreatedBy = credential, createdBy
	image, mirror := ManagedRegistryAssociations(registry)
	if err := s.repository.CreateManagedOCIRegistry(registry, image, mirror, projectIDs); err != nil {
		return fmt.Errorf("保存制品库配置失败，请检查名称、地址和项目授权: %w", err)
	}
	return nil
}

func (s *ManagedRegistryService) ResolveUpdatePassword(registry *model.ManagedOCIRegistry, suppliedPassword string) (string, error) {
	password := strings.TrimSpace(suppliedPassword)
	var err error
	if password != "" {
		registry.EncryptedCredential, err = EncryptCredential(s.encKey, password)
	} else {
		password, err = DecryptCredential(s.encKey, registry.EncryptedCredential)
	}
	if err != nil {
		return "", fmt.Errorf("读取或加密制品库凭据失败: %w", err)
	}
	return password, nil
}

func (s *ManagedRegistryService) ResolveStoredPassword(registry *model.ManagedOCIRegistry) (string, error) {
	password, err := DecryptCredential(s.encKey, registry.EncryptedCredential)
	if err != nil {
		return "", fmt.Errorf("读取制品库凭据失败: %w", err)
	}
	return password, nil
}

func (s *ManagedRegistryService) UpdateAssociations(registry *model.ManagedOCIRegistry, projectIDs []uint) error {
	if registry.ImageRegistryID == nil || registry.NodeRegistryMirrorID == nil {
		return errors.New("受管制品库关联记录缺失")
	}
	image, mirror := ManagedRegistryAssociations(registry)
	image.ID, mirror.ID = *registry.ImageRegistryID, *registry.NodeRegistryMirrorID
	if err := s.repository.UpdateImageRegistry(image, projectIDs); err != nil {
		return err
	}
	return s.repository.UpdateNodeRegistryMirror(mirror)
}

func (s *ManagedRegistryService) Save(registry *model.ManagedOCIRegistry) error {
	return s.repository.UpdateManagedOCIRegistry(registry)
}

// RenderNodeMirrorConfig reads all enabled mirrors, decrypts credentials at the
// service boundary, and renders the K3s configuration consumed by node agents.
func (s *ManagedRegistryService) RenderNodeMirrorConfig() ([]byte, error) {
	mirrors, err := s.repository.ListNodeRegistryMirrors()
	if err != nil {
		return nil, err
	}
	return RenderK3sRegistriesWithStoredCredentials(mirrors, s.encKey)
}

func (s *ManagedRegistryService) Delete(id uint) (*model.ManagedOCIRegistry, error) {
	registry, err := s.PrepareDelete(id)
	if err != nil {
		return nil, err
	}
	if err := s.DeletePersisted(registry); err != nil {
		return nil, err
	}
	return registry, nil
}

func (s *ManagedRegistryService) PrepareDelete(id uint) (*model.ManagedOCIRegistry, error) {
	registry, err := s.repository.GetManagedOCIRegistry(id)
	if err != nil {
		return nil, err
	}
	releases, defaults, err := s.repository.CountManagedOCIRegistryReferences(id)
	if err != nil {
		return nil, fmt.Errorf("检查制品库引用失败: %w", err)
	}
	if releases > 0 || defaults > 0 {
		return nil, fmt.Errorf("制品库仍被 %d 个发布记录和 %d 个项目默认仓库引用", releases, defaults)
	}
	return registry, nil
}

func (s *ManagedRegistryService) DeletePersisted(registry *model.ManagedOCIRegistry) error {
	if registry.NodeRegistryMirrorID != nil {
		_ = s.repository.DeleteNodeRegistryMirror(*registry.NodeRegistryMirrorID)
	}
	if registry.ImageRegistryID != nil {
		_ = s.repository.DeleteImageRegistry(*registry.ImageRegistryID)
	}
	if err := s.repository.DeleteManagedOCIRegistry(registry.ID); err != nil {
		return fmt.Errorf("删除受管制品库记录失败: %w", err)
	}
	return nil
}
