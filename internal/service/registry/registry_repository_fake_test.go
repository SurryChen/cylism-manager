package registry

import (
	"context"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	"gorm.io/gorm"
)

type managedRegistryRepositoryFake struct {
	registries []model.ManagedOCIRegistry
	images     map[string]model.ImageRegistry
	mirrors    map[string]model.NodeRegistryMirror
	releases   int64
	defaults   int64
	created    struct {
		registry model.ManagedOCIRegistry
		image    model.ImageRegistry
		mirror   model.NodeRegistryMirror
		projects []uint
	}
}

var _ repository.ManagedRegistryRepository = (*managedRegistryRepositoryFake)(nil)

func (f *managedRegistryRepositoryFake) ListManagedOCIRegistries() ([]model.ManagedOCIRegistry, error) {
	return append([]model.ManagedOCIRegistry(nil), f.registries...), nil
}
func (f *managedRegistryRepositoryFake) GetManagedOCIRegistry(id uint) (*model.ManagedOCIRegistry, error) {
	for index := range f.registries {
		if f.registries[index].ID == id {
			item := f.registries[index]
			return &item, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}
func (f *managedRegistryRepositoryFake) GetManagedOCIRegistryByEndpoint(endpoint string) (*model.ManagedOCIRegistry, error) {
	for index := range f.registries {
		if f.registries[index].Endpoint == endpoint {
			item := f.registries[index]
			return &item, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}
func (f *managedRegistryRepositoryFake) CreateManagedOCIRegistry(registry *model.ManagedOCIRegistry, image *model.ImageRegistry, mirror *model.NodeRegistryMirror, projects []uint) error {
	registry.ID, image.ID, mirror.ID = 10, 11, 12
	image.ManagedRegistryID, mirror.ManagedRegistryID = &registry.ID, &registry.ID
	registry.ImageRegistryID, registry.NodeRegistryMirrorID = &image.ID, &mirror.ID
	f.created.registry, f.created.image, f.created.mirror = *registry, *image, *mirror
	f.created.projects = append([]uint(nil), projects...)
	f.registries = append(f.registries, *registry)
	return nil
}
func (f *managedRegistryRepositoryFake) UpdateManagedOCIRegistry(registry *model.ManagedOCIRegistry) error {
	return nil
}
func (f *managedRegistryRepositoryFake) DeleteManagedOCIRegistry(id uint) error { return nil }
func (f *managedRegistryRepositoryFake) CountManagedOCIRegistryReferences(uint) (int64, int64, error) {
	return f.releases, f.defaults, nil
}
func (f *managedRegistryRepositoryFake) GetImageRegistryByEndpoint(endpoint string) (*model.ImageRegistry, error) {
	item, ok := f.images[endpoint]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return &item, nil
}
func (f *managedRegistryRepositoryFake) GetNodeRegistryMirrorByRegistry(registry string) (*model.NodeRegistryMirror, error) {
	item, ok := f.mirrors[registry]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return &item, nil
}
func (f *managedRegistryRepositoryFake) GetNodeRegistryMirror(uint) (*model.NodeRegistryMirror, error) {
	return nil, gorm.ErrRecordNotFound
}
func (f *managedRegistryRepositoryFake) ListNodeRegistryMirrors() ([]model.NodeRegistryMirror, error) {
	return nil, nil
}
func (f *managedRegistryRepositoryFake) ListServers() ([]model.Server, error) { return nil, nil }
func (f *managedRegistryRepositoryFake) UpsertNodeRegistryMirrorStatus(*model.NodeRegistryMirrorNode) error {
	return nil
}
func (f *managedRegistryRepositoryFake) UpdateImageRegistry(*model.ImageRegistry, []uint) error {
	return nil
}
func (f *managedRegistryRepositoryFake) UpdateNodeRegistryMirror(*model.NodeRegistryMirror) error {
	return nil
}
func (f *managedRegistryRepositoryFake) DeleteImageRegistry(uint) error      { return nil }
func (f *managedRegistryRepositoryFake) DeleteNodeRegistryMirror(uint) error { return nil }

func TestManagedRegistryServicePersistsThroughRepositoryContract(t *testing.T) {
	repo := &managedRegistryRepositoryFake{}
	service := NewManagedRegistryService(repo, []byte("01234567890123456789012345678901"))
	registry := &model.ManagedOCIRegistry{
		Name: "platform", ResourceName: "platform-registry", Endpoint: "registry.example.com",
		PullUsername: "pull", VerificationImage: "registry.example.com/platform:1",
	}
	if err := service.PersistCreate(registry, "secret-password", 7, []uint{3, 5}); err != nil {
		t.Fatal(err)
	}
	if repo.created.registry.CreatedBy != 7 || repo.created.registry.EncryptedCredential == "" || repo.created.registry.EncryptedCredential == "secret-password" {
		t.Fatalf("registry credential was not encrypted before persistence: %#v", repo.created.registry)
	}
	if repo.created.image.ManagedRegistryID == nil || repo.created.mirror.ManagedRegistryID == nil || len(repo.created.projects) != 2 {
		t.Fatalf("managed registry associations were not persisted: %#v", repo.created)
	}
}

func TestManagedRegistryServiceProtectsReferencedRegistryThroughRepositoryContract(t *testing.T) {
	repo := &managedRegistryRepositoryFake{registries: []model.ManagedOCIRegistry{{ID: 10}}, releases: 1}
	service := NewManagedRegistryService(repo, nil)
	if _, err := service.PrepareDelete(10); err == nil {
		t.Fatal("expected delete to be blocked by a release reference")
	}
}

type nodeMirrorRepositoryFake struct {
	mirrors map[uint]model.NodeRegistryMirror
}

var _ repository.NodeRegistryMirrorRepository = (*nodeMirrorRepositoryFake)(nil)

func (f *nodeMirrorRepositoryFake) CreateNodeRegistryMirror(mirror *model.NodeRegistryMirror) error {
	mirror.ID = 1
	if f.mirrors == nil {
		f.mirrors = make(map[uint]model.NodeRegistryMirror)
	}
	f.mirrors[mirror.ID] = *mirror
	return nil
}
func (f *nodeMirrorRepositoryFake) GetNodeRegistryMirror(id uint) (*model.NodeRegistryMirror, error) {
	mirror, ok := f.mirrors[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return &mirror, nil
}
func (f *nodeMirrorRepositoryFake) ListNodeRegistryMirrors() ([]model.NodeRegistryMirror, error) {
	result := make([]model.NodeRegistryMirror, 0, len(f.mirrors))
	for _, mirror := range f.mirrors {
		result = append(result, mirror)
	}
	return result, nil
}
func (f *nodeMirrorRepositoryFake) UpdateNodeRegistryMirror(mirror *model.NodeRegistryMirror) error {
	f.mirrors[mirror.ID] = *mirror
	return nil
}
func (f *nodeMirrorRepositoryFake) UpdateNodeRegistryMirrorVerification(id uint, status, detail string, verifiedAt time.Time) error {
	mirror, ok := f.mirrors[id]
	if !ok {
		return gorm.ErrRecordNotFound
	}
	mirror.LastVerifiedAt, mirror.LastVerifyStatus, mirror.LastVerifyError = &verifiedAt, status, detail
	f.mirrors[id] = mirror
	return nil
}
func (f *nodeMirrorRepositoryFake) DeleteNodeRegistryMirror(id uint) error {
	delete(f.mirrors, id)
	return nil
}
func (f *nodeMirrorRepositoryFake) UpsertNodeRegistryMirrorStatus(*model.NodeRegistryMirrorNode) error {
	return nil
}
func (f *nodeMirrorRepositoryFake) ListServers() ([]model.Server, error) { return nil, nil }

func TestMirrorServicePersistsAndVerifiesThroughRepositoryContract(t *testing.T) {
	repo := &nodeMirrorRepositoryFake{}
	service := NewMirrorService(repo, []byte("01234567890123456789012345678901"))
	mirror, err := service.Create(MirrorInput{
		Name: "docker-hub", Registry: "docker.io", Endpoints: []string{"https://mirror.example.com"},
		VerificationImage: "docker.io/library/busybox:1.36", Username: "robot", Credential: "secret",
	}, 8)
	if err != nil {
		t.Fatal(err)
	}
	if mirror.Credential != "" || !mirror.CredentialConfigured || repo.mirrors[mirror.ID].Credential == "" {
		t.Fatalf("mirror credentials were not safely persisted: response=%#v stored=%#v", mirror, repo.mirrors[mirror.ID])
	}
	verified, err := service.Verify(context.Background(), mirror.ID, func(context.Context, *model.NodeRegistryMirror) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if verified.LastVerifyStatus != "succeeded" || verified.LastVerifiedAt == nil {
		t.Fatalf("mirror verification was not persisted: %#v", verified)
	}
}
