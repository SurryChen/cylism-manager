// Package cloud owns provider-neutral DNS and object storage workflows.
package cloud

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	security "github.com/cylism/cylism-manager/internal/security"
)

type DNSZone struct {
	Name       string `json:"name"`
	InstanceID string `json:"instance_id,omitempty"`
}
type DNSRecord struct {
	ID       string `json:"id"`
	RR       string `json:"rr"`
	Type     string `json:"type"`
	Line     string `json:"line"`
	Value    string `json:"value"`
	TTL      int64  `json:"ttl"`
	Priority int64  `json:"priority,omitempty"`
	Enabled  bool   `json:"enabled"`
}
type DNSRecordPage struct {
	Records  []DNSRecord `json:"records"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	Total    int64       `json:"total"`
}
type DNSRecordInput struct {
	RR       string `json:"rr"`
	Type     string `json:"type"`
	Line     string `json:"line"`
	Value    string `json:"value"`
	TTL      int64  `json:"ttl"`
	Priority int64  `json:"priority,omitempty"`
}
type Object struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	ETag         string    `json:"etag,omitempty"`
	LastModified time.Time `json:"last_modified,omitempty"`
	ContentType  string    `json:"content_type,omitempty"`
}
type ObjectPage struct {
	Objects   []Object `json:"objects"`
	NextToken string   `json:"next_token,omitempty"`
	Truncated bool     `json:"truncated"`
}
type ObjectDownload struct {
	Body        io.ReadCloser
	ContentType string
	Size        int64
	Filename    string
}

// Container is a provider-neutral object storage container.
type Container struct {
	Name         string    `json:"name"`
	Region       string    `json:"region,omitempty"`
	StorageClass string    `json:"storage_class,omitempty"`
	ACL          string    `json:"acl,omitempty"`
	Versioning   string    `json:"versioning,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
}
type ContainerInput struct {
	Name         string `json:"name"`
	Region       string `json:"region,omitempty"`
	StorageClass string `json:"storage_class,omitempty"`
	ACL          string `json:"acl,omitempty"`
	Versioning   string `json:"versioning,omitempty"`
}

// Provider is a marker; adapters may implement one or both capability interfaces.
type Provider interface{}

type DNSProvider interface {
	ValidateDNS(context.Context) error
	ListZones(context.Context) ([]DNSZone, error)
	ListRecords(context.Context, string, int, int) (DNSRecordPage, error)
	CreateRecord(context.Context, string, DNSRecordInput) (DNSRecord, error)
	UpdateRecord(context.Context, string, DNSRecordInput) (DNSRecord, error)
	DeleteRecord(context.Context, string) error
}

type ObjectStorageProvider interface {
	ValidateObjectStorage(context.Context) error
	ListContainers(context.Context) ([]Container, error)
	CreateContainer(context.Context, ContainerInput) (Container, error)
	UpdateContainer(context.Context, string, ContainerInput) (Container, error)
	ContainerEmpty(context.Context, string) (bool, error)
	DeleteContainer(context.Context, string) error
	ListObjects(context.Context, string, string, string) (ObjectPage, error)
	UploadObject(context.Context, string, string, io.Reader, int64, string) (Object, error)
	DownloadObject(context.Context, string, string) (ObjectDownload, error)
	DeleteObject(context.Context, string, string) error
}

type ProviderFactory func(credentials, configuration string) (Provider, error)
type ProviderRegistry map[string]ProviderFactory

type Service struct {
	repository repository.CloudConnectionRepository
	key        []byte
	providers  ProviderRegistry
}

func NewService(repo repository.CloudConnectionRepository, key []byte, providers ProviderRegistry) *Service {
	copied := make(ProviderRegistry, len(providers))
	for name, factory := range providers {
		copied[strings.ToLower(strings.TrimSpace(name))] = factory
	}
	return &Service{repository: repo, key: append([]byte(nil), key...), providers: copied}
}

func (s *Service) ListConnections() ([]model.CloudConnection, error) {
	return s.repository.ListCloudConnections()
}
func (s *Service) ProviderCatalog() []ProviderSpec { return ProviderCatalog(s.providers) }
func (s *Service) GetConnection(id uint) (*model.CloudConnection, error) {
	return s.repository.GetCloudConnection(id)
}
func (s *Service) SaveConnection(connection *model.CloudConnection, credentials *string) error {
	connection.Name = strings.TrimSpace(connection.Name)
	connection.Provider = strings.ToLower(strings.TrimSpace(connection.Provider))
	connection.Configuration = strings.TrimSpace(connection.Configuration)
	if connection.Name == "" || connection.Provider == "" {
		return errors.New("连接名称和云提供商必填")
	}
	factory, ok := s.providers[connection.Provider]
	if !ok || factory == nil {
		return errors.New("不支持的云提供商")
	}
	if credentials != nil {
		if strings.TrimSpace(*credentials) == "" {
			return errors.New("云提供商凭据不能为空")
		}
		ciphertext, err := security.Encrypt(s.key, strings.TrimSpace(*credentials))
		if err != nil {
			return fmt.Errorf("加密云提供商凭据: %w", err)
		}
		connection.EncryptedCredentials = ciphertext
	}
	if connection.EncryptedCredentials == "" {
		return errors.New("云提供商凭据必填")
	}
	plaintext, err := security.Decrypt(s.key, connection.EncryptedCredentials)
	if err != nil {
		return errors.New("读取云提供商凭据失败")
	}
	provider, err := factory(plaintext, connection.Configuration)
	if err != nil {
		return safeProviderError(err)
	}
	_, connection.DNSEnabled = provider.(DNSProvider)
	_, connection.ObjectStorageEnabled = provider.(ObjectStorageProvider)
	if !connection.DNSEnabled && !connection.ObjectStorageEnabled {
		return errors.New("该云提供商尚未实现域名管理或对象存储能力")
	}
	if connection.ID == 0 {
		return s.repository.CreateCloudConnection(connection)
	}
	return s.repository.UpdateCloudConnection(connection)
}
func (s *Service) DeleteConnection(id uint) error { return s.repository.DeleteCloudConnection(id) }

func (s *Service) Validate(ctx context.Context, id uint) (*model.CloudConnection, error) {
	connection, provider, err := s.provider(id)
	if err != nil {
		return nil, err
	}
	dnsStatus, storageStatus, detail := "disabled", "disabled", ""
	if _, ok := provider.(DNSProvider); ok {
		dnsStatus = "ready"
		p := provider.(DNSProvider)
		if err := p.ValidateDNS(ctx); err != nil {
			dnsStatus, detail = "failed", safeProviderError(err).Error()
		}
	}
	if _, ok := provider.(ObjectStorageProvider); ok {
		storageStatus = "ready"
		p := provider.(ObjectStorageProvider)
		if err := p.ValidateObjectStorage(ctx); err != nil {
			storageStatus = "failed"
			if detail == "" {
				detail = safeProviderError(err).Error()
			}
		}
	}
	now := time.Now().UTC()
	if err := s.repository.UpdateCloudConnectionValidation(id, dnsStatus, storageStatus, detail, now); err != nil {
		return nil, err
	}
	connection.DNSStatus, connection.ObjectStorageStatus, connection.LastValidationError, connection.LastValidationAt = dnsStatus, storageStatus, detail, &now
	return connection, nil
}
func (s *Service) ListZones(ctx context.Context, id uint) ([]DNSZone, error) {
	_, p, err := s.dnsProvider(id)
	if err != nil {
		return nil, err
	}
	return p.ListZones(ctx)
}
func (s *Service) ListRecords(ctx context.Context, id uint, zone string, page, pageSize int) (DNSRecordPage, error) {
	_, p, err := s.dnsProvider(id)
	if err != nil {
		return DNSRecordPage{}, err
	}
	return p.ListRecords(ctx, strings.TrimSpace(zone), page, pageSize)
}
func (s *Service) CreateRecord(ctx context.Context, id uint, zone string, input DNSRecordInput) (DNSRecord, error) {
	if err := validateRecord(input); err != nil {
		return DNSRecord{}, err
	}
	_, p, err := s.dnsProvider(id)
	if err != nil {
		return DNSRecord{}, err
	}
	return p.CreateRecord(ctx, strings.TrimSpace(zone), input)
}
func (s *Service) UpdateRecord(ctx context.Context, id uint, recordID string, input DNSRecordInput) (DNSRecord, error) {
	if strings.TrimSpace(recordID) == "" {
		return DNSRecord{}, errors.New("记录 ID 必填")
	}
	if err := validateRecord(input); err != nil {
		return DNSRecord{}, err
	}
	_, p, err := s.dnsProvider(id)
	if err != nil {
		return DNSRecord{}, err
	}
	return p.UpdateRecord(ctx, recordID, input)
}
func (s *Service) DeleteRecord(ctx context.Context, id uint, recordID string, confirmed bool) error {
	if !confirmed {
		return errors.New("请确认删除 DNS 记录")
	}
	if strings.TrimSpace(recordID) == "" {
		return errors.New("记录 ID 必填")
	}
	_, p, err := s.dnsProvider(id)
	if err != nil {
		return err
	}
	return p.DeleteRecord(ctx, recordID)
}
func (s *Service) ListContainers(ctx context.Context, id uint) ([]Container, error) {
	_, p, err := s.objectStorageProvider(id)
	if err != nil {
		return nil, err
	}
	return p.ListContainers(ctx)
}
func (s *Service) CreateContainer(ctx context.Context, id uint, input ContainerInput) (Container, error) {
	if strings.TrimSpace(input.Name) == "" {
		return Container{}, errors.New("容器名称必填")
	}
	_, p, err := s.objectStorageProvider(id)
	if err != nil {
		return Container{}, err
	}
	return p.CreateContainer(ctx, input)
}
func (s *Service) UpdateContainer(ctx context.Context, id uint, name string, input ContainerInput) (Container, error) {
	if strings.TrimSpace(name) == "" {
		return Container{}, errors.New("容器名称必填")
	}
	_, p, err := s.objectStorageProvider(id)
	if err != nil {
		return Container{}, err
	}
	return p.UpdateContainer(ctx, name, input)
}
func (s *Service) DeleteContainer(ctx context.Context, id uint, name string, confirmed bool) error {
	if !confirmed {
		return errors.New("请确认删除存储容器")
	}
	_, p, err := s.objectStorageProvider(id)
	if err != nil {
		return err
	}
	empty, err := p.ContainerEmpty(ctx, name)
	if err != nil {
		return err
	}
	if !empty {
		return errors.New("存储容器非空，不能删除")
	}
	return p.DeleteContainer(ctx, name)
}
func (s *Service) ListObjects(ctx context.Context, id uint, container, prefix, token string) (ObjectPage, error) {
	_, p, err := s.objectStorageProvider(id)
	if err != nil {
		return ObjectPage{}, err
	}
	return p.ListObjects(ctx, container, prefix, token)
}
func (s *Service) UploadObject(ctx context.Context, id uint, container, key string, body io.Reader, size int64, contentType string) (Object, error) {
	if size < 0 || size > 1024*1024*1024 {
		return Object{}, errors.New("文件大小超出 1GiB 限制")
	}
	_, p, err := s.objectStorageProvider(id)
	if err != nil {
		return Object{}, err
	}
	return p.UploadObject(ctx, container, key, body, size, contentType)
}
func (s *Service) DownloadObject(ctx context.Context, id uint, container, key string) (ObjectDownload, error) {
	_, p, err := s.objectStorageProvider(id)
	if err != nil {
		return ObjectDownload{}, err
	}
	return p.DownloadObject(ctx, container, key)
}
func (s *Service) DeleteObject(ctx context.Context, id uint, container, key string, confirmed bool) error {
	if !confirmed {
		return errors.New("请确认删除对象")
	}
	_, p, err := s.objectStorageProvider(id)
	if err != nil {
		return err
	}
	return p.DeleteObject(ctx, container, key)
}

func (s *Service) provider(id uint) (*model.CloudConnection, Provider, error) {
	if s == nil || s.repository == nil {
		return nil, nil, errors.New("云资源服务不可用")
	}
	connection, err := s.repository.GetCloudConnection(id)
	if err != nil {
		return nil, nil, err
	}
	factory, ok := s.providers[strings.ToLower(connection.Provider)]
	if !ok || factory == nil {
		return nil, nil, errors.New("该云提供商未配置")
	}
	credentials, err := security.Decrypt(s.key, connection.EncryptedCredentials)
	if err != nil {
		return nil, nil, errors.New("读取云提供商凭据失败")
	}
	provider, err := factory(credentials, connection.Configuration)
	if err != nil {
		return nil, nil, safeProviderError(err)
	}
	return connection, provider, nil
}
func (s *Service) dnsProvider(id uint) (*model.CloudConnection, DNSProvider, error) {
	c, provider, err := s.provider(id)
	if err != nil {
		return nil, nil, err
	}
	p, ok := provider.(DNSProvider)
	if !ok {
		return nil, nil, errors.New("该云提供商未实现域名管理能力")
	}
	return c, p, nil
}
func (s *Service) objectStorageProvider(id uint) (*model.CloudConnection, ObjectStorageProvider, error) {
	c, provider, err := s.provider(id)
	if err != nil {
		return nil, nil, err
	}
	p, ok := provider.(ObjectStorageProvider)
	if !ok {
		return nil, nil, errors.New("该云提供商未实现对象存储能力")
	}
	return c, p, nil
}
func validateRecord(input DNSRecordInput) error {
	input.Type = strings.ToUpper(strings.TrimSpace(input.Type))
	if input.RR == "" || input.Type == "" || strings.TrimSpace(input.Value) == "" {
		return errors.New("主机记录、类型和值必填")
	}
	if input.TTL < 1 || input.TTL > 86400 {
		return errors.New("TTL 必须在 1 到 86400 秒之间")
	}
	return nil
}
func safeProviderError(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(security.Truncate(security.Redact(strings.TrimSpace(err.Error())), 480))
}
