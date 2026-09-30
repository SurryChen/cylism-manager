package cloud

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	security "github.com/cylism/cylism-manager/internal/security"
)

type fakeRepository struct{ connection *model.CloudConnection }

func (r *fakeRepository) CreateCloudConnection(item *model.CloudConnection) error {
	r.connection = item
	item.ID = 1
	return nil
}
func (r *fakeRepository) ListCloudConnections() ([]model.CloudConnection, error) { return nil, nil }
func (r *fakeRepository) GetCloudConnection(uint) (*model.CloudConnection, error) {
	if r.connection == nil {
		return nil, errors.New("not found")
	}
	copy := *r.connection
	return &copy, nil
}
func (r *fakeRepository) UpdateCloudConnection(item *model.CloudConnection) error {
	r.connection = item
	return nil
}
func (r *fakeRepository) DeleteCloudConnection(uint) error { return nil }

type fakeProvider struct {
	deleteCalled bool
	region       string
}

func (p *fakeProvider) SetStorageRegion(region string) error { p.region = region; return nil }

func (p *fakeProvider) ListZones(context.Context) ([]DNSZone, error) { return nil, nil }
func (p *fakeProvider) ListRecords(context.Context, string, int, int) (DNSRecordPage, error) {
	return DNSRecordPage{}, nil
}
func (p *fakeProvider) CreateRecord(context.Context, string, DNSRecordInput) (DNSRecord, error) {
	return DNSRecord{}, nil
}
func (p *fakeProvider) UpdateRecord(context.Context, string, DNSRecordInput) (DNSRecord, error) {
	return DNSRecord{}, nil
}
func (p *fakeProvider) DeleteRecord(context.Context, string) error          { p.deleteCalled = true; return nil }
func (p *fakeProvider) ListContainers(context.Context) ([]Container, error) { return nil, nil }
func (p *fakeProvider) CreateContainer(context.Context, ContainerInput) (Container, error) {
	return Container{}, nil
}
func (p *fakeProvider) UpdateContainer(context.Context, string, ContainerInput) (Container, error) {
	return Container{}, nil
}
func (p *fakeProvider) ContainerEmpty(context.Context, string) (bool, error) { return true, nil }
func (p *fakeProvider) DeleteContainer(context.Context, string) error        { return nil }
func (p *fakeProvider) ListObjects(context.Context, string, string, string) (ObjectPage, error) {
	return ObjectPage{}, nil
}
func (p *fakeProvider) UploadObject(context.Context, string, string, io.Reader, int64, string) (Object, error) {
	return Object{}, nil
}
func (p *fakeProvider) DownloadObject(context.Context, string, string) (ObjectDownload, error) {
	return ObjectDownload{}, nil
}
func (p *fakeProvider) DeleteObject(context.Context, string, string) error { return nil }

func newTestService(t *testing.T, connection *model.CloudConnection, provider *fakeProvider) *Service {
	t.Helper()
	key := []byte("0123456789abcdef0123456789abcdef")
	ciphertext, err := security.Encrypt(key, `{"token":"secret"}`)
	if err != nil {
		t.Fatal(err)
	}
	connection.EncryptedCredentials = ciphertext
	return NewService(&fakeRepository{connection: connection}, key, ProviderRegistry{"example": func(string, string) (Provider, error) { return provider, nil }})
}

func TestServiceUsesProviderInterfacesForDNSCapability(t *testing.T) {
	service := newTestService(t, &model.CloudConnection{ID: 1, Provider: "example", ObjectStorageEnabled: true}, &fakeProvider{})
	if _, err := service.ListZones(context.Background(), 1); err != nil {
		t.Fatalf("ListZones error = %v, provider capability should be detected automatically", err)
	}
}

func TestServiceRequiresConfirmationForRecordDelete(t *testing.T) {
	provider := &fakeProvider{}
	service := newTestService(t, &model.CloudConnection{ID: 1, Provider: "example", DNSEnabled: true}, provider)
	if err := service.DeleteRecord(context.Background(), 1, "123", false); err == nil {
		t.Fatal("delete without confirmation must fail")
	}
	if provider.deleteCalled {
		t.Fatal("delete must not reach provider")
	}
	if err := service.DeleteRecord(context.Background(), 1, "123", true); err != nil {
		t.Fatal(err)
	}
	if !provider.deleteCalled {
		t.Fatal("confirmed delete must reach provider")
	}
}

func TestObjectStorageUsesRegionForEachOperation(t *testing.T) {
	provider := &fakeProvider{}
	service := newTestService(t, &model.CloudConnection{ID: 1, Provider: "example"}, provider)
	if _, err := service.CreateContainer(context.Background(), 1, ContainerInput{Name: "assets"}); err == nil {
		t.Fatal("regional object storage requires an explicit creation region")
	}
	if _, err := service.CreateContainer(context.Background(), 1, ContainerInput{Name: "assets", Region: "cn-shanghai"}); err != nil || provider.region != "cn-shanghai" {
		t.Fatalf("create region = %q, err = %v", provider.region, err)
	}
	if _, err := service.ListObjects(context.Background(), 1, "assets", "", "", "cn-beijing"); err != nil || provider.region != "cn-beijing" {
		t.Fatalf("list region = %q, err = %v", provider.region, err)
	}
}

func TestServiceSavesGenericCredentialsWithoutRetainingPlaintext(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	repo := &fakeRepository{}
	service := NewService(repo, key, ProviderRegistry{"example": func(string, string) (Provider, error) { return &fakeProvider{}, nil }})
	credentials := `{"api_key":"secret"}`
	connection := &model.CloudConnection{Name: "生产云连接", Provider: "example"}
	if err := service.SaveConnection(connection, &credentials); err != nil {
		t.Fatal(err)
	}
	if repo.connection == nil || repo.connection.EncryptedCredentials == credentials {
		t.Fatal("credentials were not encrypted")
	}
	if !repo.connection.DNSEnabled || !repo.connection.ObjectStorageEnabled {
		t.Fatal("provider interfaces should determine capabilities")
	}
}

type fakePermissionProvider struct{ result PermissionInspection }

func (p *fakePermissionProvider) InspectPermissions(context.Context) PermissionInspection {
	return p.result
}

func TestInspectPermissionsUsesConnectionCredentialsWithoutReturningThem(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	ciphertext, err := security.Encrypt(key, `{"access_key_secret":"secret-value"}`)
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeRepository{connection: &model.CloudConnection{ID: 7, Provider: "aliyun", EncryptedCredentials: ciphertext}}
	service := NewService(repo, key, ProviderRegistry{"aliyun": func(credentials, _ string) (Provider, error) {
		if credentials != `{"access_key_secret":"secret-value"}` {
			t.Fatalf("factory credentials = %q", credentials)
		}
		return &fakePermissionProvider{result: PermissionInspection{
			Status: "complete", Identity: &PermissionIdentity{Type: "RAMUser", ARN: "acs:ram::123:user/cylism"},
			Policies: []AssignedPolicy{{Name: "AliyunDNSFullAccess", Type: "System", Source: "direct"}},
		}}, nil
	}})
	result, err := service.InspectPermissions(context.Background(), 7)
	if err != nil || result.Status != "complete" || len(result.Policies) != 1 {
		t.Fatalf("inspection = %#v, %v", result, err)
	}
	if result.InspectedAt.IsZero() {
		t.Fatal("inspection time missing")
	}
}

func TestInspectPermissionsReportsUnsupportedProvider(t *testing.T) {
	service := newTestService(t, &model.CloudConnection{ID: 1, Provider: "example"}, &fakeProvider{})
	result, err := service.InspectPermissions(context.Background(), 1)
	if err != nil || result.Status != "unsupported" {
		t.Fatalf("inspection = %#v, %v", result, err)
	}
}
