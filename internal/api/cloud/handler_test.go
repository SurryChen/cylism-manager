package cloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	cloudservice "github.com/cylism/cylism-manager/internal/service/cloud"
	"github.com/gin-gonic/gin"
)

func TestConnectionCreateDoesNotReturnSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, repo := testService(t)
	handler := NewHandler(service, nil)
	router := gin.New()
	router.POST("/connections", handler.CreateConnection)
	recorder := perform(router, http.MethodPost, "/connections", `{"name":"生产云连接","provider":"example","credentials":"{\"api_key\":\"secret-value\"}","configuration":"{}","dns_enabled":false,"object_storage_enabled":true}`)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "secret-value") {
		t.Fatalf("secret leaked: %s", recorder.Body.String())
	}
	if repo.item == nil || !repo.item.CredentialConfigured {
		t.Fatalf("connection not saved: %#v", repo.item)
	}
	if !repo.item.DNSEnabled || repo.item.ObjectStorageEnabled {
		t.Fatalf("connection capabilities should be automatic: %#v", repo.item)
	}
}

func TestListProvidersReturnsConfigurationCatalog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, _ := testService(t)
	handler := NewHandler(service, nil)
	router := gin.New()
	router.GET("/providers", handler.ListProviders)
	recorder := perform(router, http.MethodGet, "/providers", "")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "aliyun") || !strings.Contains(recorder.Body.String(), "tencent") {
		t.Fatalf("provider catalog response = %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestInspectPermissionsReturnsSafeAssignments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service, repo := testService(t)
	secret := `{"api_key":"secret-value"}`
	repo.item = &model.CloudConnection{ID: 1, Name: "生产连接", Provider: "example"}
	if err := service.SaveConnection(repo.item, &secret); err != nil {
		t.Fatal(err)
	}
	audit := &permissionAuditRepository{}
	router := gin.New()
	router.GET("/connections/:id/permissions", NewHandler(service, audit).InspectPermissions)
	recorder := perform(router, http.MethodGet, "/connections/1/permissions", "")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"complete"`) {
		t.Fatalf("inspection response = %d: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "secret-value") || strings.Contains(recorder.Body.String(), "api_key") {
		t.Fatalf("credential leaked: %s", recorder.Body.String())
	}
	if audit.log == nil || audit.log.Action != "cloud.connection.permissions.inspect" || strings.Contains(audit.log.Detail, "AliyunDNSFullAccess") || strings.Contains(audit.log.Detail, "secret-value") || strings.Contains(audit.log.Summary, "AliyunDNSFullAccess") {
		t.Fatalf("unsafe audit log: %#v", audit.log)
	}
}

type permissionAuditRepository struct{ log *model.AuditLog }

func (r *permissionAuditRepository) CreateAuditLog(log *model.AuditLog) error {
	r.log = log
	return nil
}

type handlerRepository struct{ item *model.CloudConnection }

func (r *handlerRepository) CreateCloudConnection(item *model.CloudConnection) error {
	item.ID = 1
	item.CredentialConfigured = item.EncryptedCredentials != ""
	r.item = item
	return nil
}
func (r *handlerRepository) ListCloudConnections() ([]model.CloudConnection, error) { return nil, nil }
func (r *handlerRepository) GetCloudConnection(uint) (*model.CloudConnection, error) {
	return r.item, nil
}
func (r *handlerRepository) UpdateCloudConnection(item *model.CloudConnection) error {
	r.item = item
	return nil
}
func (r *handlerRepository) DeleteCloudConnection(uint) error { return nil }

func testService(t *testing.T) (*cloudservice.Service, *handlerRepository) {
	t.Helper()
	repo := &handlerRepository{}
	providers := cloudservice.ProviderRegistry{"example": func(string, string) (cloudservice.Provider, error) { return &testProvider{}, nil }}
	return cloudservice.NewService(repo, []byte("0123456789abcdef0123456789abcdef"), providers), repo
}

type testProvider struct{}

func (*testProvider) InspectPermissions(context.Context) cloudservice.PermissionInspection {
	return cloudservice.PermissionInspection{Status: "complete", Policies: []cloudservice.AssignedPolicy{{Name: "AliyunDNSFullAccess", Source: "direct"}}}
}

func (*testProvider) ListZones(context.Context) ([]cloudservice.DNSZone, error) { return nil, nil }
func (*testProvider) ListRecords(context.Context, string, int, int) (cloudservice.DNSRecordPage, error) {
	return cloudservice.DNSRecordPage{}, nil
}
func (*testProvider) CreateRecord(context.Context, string, cloudservice.DNSRecordInput) (cloudservice.DNSRecord, error) {
	return cloudservice.DNSRecord{}, nil
}
func (*testProvider) UpdateRecord(context.Context, string, cloudservice.DNSRecordInput) (cloudservice.DNSRecord, error) {
	return cloudservice.DNSRecord{}, nil
}
func (*testProvider) DeleteRecord(context.Context, string) error { return nil }

func perform(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}
