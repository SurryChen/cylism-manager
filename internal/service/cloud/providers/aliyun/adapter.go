package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/alidns"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ram"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/sts"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	cloudservice "github.com/cylism/cylism-manager/internal/service/cloud"
)

type Provider = cloudservice.Provider
type DNSZone = cloudservice.DNSZone
type DNSRecord = cloudservice.DNSRecord
type DNSRecordPage = cloudservice.DNSRecordPage
type DNSRecordInput = cloudservice.DNSRecordInput
type Container = cloudservice.Container
type ContainerInput = cloudservice.ContainerInput
type Object = cloudservice.Object
type ObjectPage = cloudservice.ObjectPage
type ObjectDownload = cloudservice.ObjectDownload

type aliyunProvider struct {
	dns             *alidns.Client
	oss             *oss.Client
	caller          callerIdentityClient
	ram             ramPolicyClient
	region          string
	accessKeyID     string
	accessKeySecret string
}

var storageRegionPattern = regexp.MustCompile(`^[a-z]{2}-[a-z0-9]+(?:-[a-z0-9]+)*$`)

type callerIdentityClient interface {
	GetCallerIdentity(*sts.GetCallerIdentityRequest) (*sts.GetCallerIdentityResponse, error)
}

type ramPolicyClient interface {
	ListPoliciesForUser(*ram.ListPoliciesForUserRequest) (*ram.ListPoliciesForUserResponse, error)
	ListGroupsForUser(*ram.ListGroupsForUserRequest) (*ram.ListGroupsForUserResponse, error)
	ListPoliciesForGroup(*ram.ListPoliciesForGroupRequest) (*ram.ListPoliciesForGroupResponse, error)
}

func NewProvider(credentialsJSON, configurationJSON string) (Provider, error) {
	var credentials struct {
		AccessKeyID     string `json:"access_key_id"`
		AccessKeySecret string `json:"access_key_secret"`
	}
	if err := json.Unmarshal([]byte(credentialsJSON), &credentials); err != nil {
		return nil, fmt.Errorf("解析阿里云凭据: %w", err)
	}
	var configuration struct {
		Region string `json:"region"`
	}
	if strings.TrimSpace(configurationJSON) != "" {
		if err := json.Unmarshal([]byte(configurationJSON), &configuration); err != nil {
			return nil, fmt.Errorf("解析阿里云配置: %w", err)
		}
	}
	accessKeyID, accessKeySecret, region := strings.TrimSpace(credentials.AccessKeyID), strings.TrimSpace(credentials.AccessKeySecret), strings.TrimSpace(configuration.Region)
	if accessKeyID == "" || accessKeySecret == "" {
		return nil, fmt.Errorf("AccessKey ID 和 AccessKey Secret 必填")
	}
	if region == "" {
		region = "cn-hangzhou"
	}
	if !storageRegionPattern.MatchString(region) {
		return nil, fmt.Errorf("存储地域格式无效")
	}
	dns, err := alidns.NewClientWithAccessKey(region, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, err
	}
	client, err := oss.New("https://oss-"+region+".aliyuncs.com", accessKeyID, accessKeySecret, oss.Region(region), oss.Timeout(10, 60))
	if err != nil {
		return nil, err
	}
	caller, err := sts.NewClientWithAccessKey(region, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, err
	}
	ramClient, err := ram.NewClientWithAccessKey(region, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, err
	}
	return &aliyunProvider{dns: dns, oss: client, caller: caller, ram: ramClient, region: region, accessKeyID: accessKeyID, accessKeySecret: accessKeySecret}, nil
}

func (p *aliyunProvider) SetStorageRegion(region string) error {
	region = strings.TrimSpace(region)
	if region == "" || region == p.region {
		return nil
	}
	if !storageRegionPattern.MatchString(region) {
		return fmt.Errorf("存储地域格式无效")
	}
	client, err := oss.New("https://oss-"+region+".aliyuncs.com", p.accessKeyID, p.accessKeySecret, oss.Region(region), oss.Timeout(10, 60))
	if err != nil {
		return err
	}
	p.oss, p.region = client, region
	return nil
}

func (p *aliyunProvider) ListZones(_ context.Context) ([]DNSZone, error) {
	request := alidns.CreateDescribeDomainsRequest()
	request.PageNumber = requests.NewInteger(1)
	request.PageSize = requests.NewInteger(100)
	response, err := p.dns.DescribeDomains(request)
	if err != nil {
		return nil, err
	}
	items := make([]DNSZone, 0, len(response.Domains.Domain))
	for _, zone := range response.Domains.Domain {
		items = append(items, DNSZone{Name: zone.DomainName, InstanceID: zone.InstanceId})
	}
	return items, nil
}

func (p *aliyunProvider) ListRecords(_ context.Context, zone string, page, pageSize int) (DNSRecordPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 100
	}
	request := alidns.CreateDescribeDomainRecordsRequest()
	request.DomainName = zone
	request.PageNumber = requests.NewInteger(page)
	request.PageSize = requests.NewInteger(pageSize)
	response, err := p.dns.DescribeDomainRecords(request)
	if err != nil {
		return DNSRecordPage{}, err
	}
	items := make([]DNSRecord, 0, len(response.DomainRecords.Record))
	for _, record := range response.DomainRecords.Record {
		items = append(items, dnsRecord(record))
	}
	return DNSRecordPage{Records: items, Page: int(response.PageNumber), PageSize: int(response.PageSize), Total: response.TotalCount}, nil
}

func (p *aliyunProvider) CreateRecord(_ context.Context, zone string, input DNSRecordInput) (DNSRecord, error) {
	request := alidns.CreateAddDomainRecordRequest()
	request.DomainName = zone
	applyRecordInput(request, input)
	response, err := p.dns.AddDomainRecord(request)
	if err != nil {
		return DNSRecord{}, err
	}
	return DNSRecord{ID: response.RecordId, RR: input.RR, Type: strings.ToUpper(input.Type), Line: recordLine(input.Line), Value: input.Value, TTL: input.TTL, Priority: input.Priority, Enabled: true}, nil
}

func (p *aliyunProvider) UpdateRecord(_ context.Context, recordID string, input DNSRecordInput) (DNSRecord, error) {
	request := alidns.CreateUpdateDomainRecordRequest()
	request.RecordId = recordID
	request.RR = input.RR
	request.Type = strings.ToUpper(input.Type)
	request.Line = recordLine(input.Line)
	request.Value = input.Value
	request.TTL = requests.NewInteger(int(input.TTL))
	request.Priority = requests.NewInteger(int(input.Priority))
	response, err := p.dns.UpdateDomainRecord(request)
	if err != nil {
		return DNSRecord{}, err
	}
	return DNSRecord{ID: response.RecordId, RR: input.RR, Type: strings.ToUpper(input.Type), Line: recordLine(input.Line), Value: input.Value, TTL: input.TTL, Priority: input.Priority, Enabled: true}, nil
}
func (p *aliyunProvider) DeleteRecord(_ context.Context, recordID string) error {
	request := alidns.CreateDeleteDomainRecordRequest()
	request.RecordId = recordID
	_, err := p.dns.DeleteDomainRecord(request)
	return err
}

func (p *aliyunProvider) ListContainers(ctx context.Context) ([]Container, error) {
	response, err := p.oss.ListBuckets(oss.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	items := make([]Container, 0, len(response.Buckets))
	for _, bucket := range response.Buckets {
		items = append(items, Container{Name: bucket.Name, Region: bucketRegion(bucket.Region, bucket.Location), StorageClass: bucket.StorageClass, CreatedAt: bucket.CreationDate})
	}
	return items, nil
}
func (p *aliyunProvider) CreateContainer(ctx context.Context, input ContainerInput) (Container, error) {
	options := []oss.Option{oss.WithContext(ctx)}
	if input.ACL != "" {
		options = append(options, oss.ACL(oss.ACLType(input.ACL)))
	}
	if input.StorageClass != "" {
		options = append(options, oss.StorageClass(oss.StorageClassType(input.StorageClass)))
	}
	if err := p.oss.CreateBucket(input.Name, options...); err != nil {
		return Container{}, err
	}
	result := Container{Name: input.Name, Region: p.region, StorageClass: input.StorageClass, ACL: input.ACL, Versioning: input.Versioning}
	if input.Versioning != "" {
		if err := p.setVersioning(ctx, input.Name, input.Versioning); err != nil {
			return Container{}, err
		}
	}
	return result, nil
}
func (p *aliyunProvider) UpdateContainer(ctx context.Context, name string, input ContainerInput) (Container, error) {
	if input.ACL != "" {
		if err := p.oss.SetBucketACL(name, oss.ACLType(input.ACL), oss.WithContext(ctx)); err != nil {
			return Container{}, err
		}
	}
	if input.Versioning != "" {
		if err := p.setVersioning(ctx, name, input.Versioning); err != nil {
			return Container{}, err
		}
	}
	return Container{Name: name, Region: p.region, ACL: input.ACL, Versioning: input.Versioning}, nil
}
func (p *aliyunProvider) ContainerEmpty(ctx context.Context, name string) (bool, error) {
	bucket, err := p.oss.Bucket(name)
	if err != nil {
		return false, err
	}
	page, err := bucket.ListObjectsV2(oss.MaxKeys(1), oss.WithContext(ctx))
	return len(page.Objects) == 0, err
}
func (p *aliyunProvider) DeleteContainer(ctx context.Context, name string) error {
	return p.oss.DeleteBucket(name, oss.WithContext(ctx))
}
func (p *aliyunProvider) ListObjects(ctx context.Context, bucketName, prefix, token string) (ObjectPage, error) {
	bucket, err := p.oss.Bucket(bucketName)
	if err != nil {
		return ObjectPage{}, err
	}
	page, err := bucket.ListObjectsV2(oss.Prefix(prefix), oss.ContinuationToken(token), oss.MaxKeys(100), oss.WithContext(ctx))
	if err != nil {
		return ObjectPage{}, err
	}
	items := make([]Object, 0, len(page.Objects))
	for _, item := range page.Objects {
		items = append(items, Object{Key: item.Key, Size: item.Size, ETag: item.ETag, LastModified: item.LastModified})
	}
	return ObjectPage{Objects: items, NextToken: page.NextContinuationToken, Truncated: page.IsTruncated}, nil
}
func (p *aliyunProvider) UploadObject(ctx context.Context, bucketName, key string, body io.Reader, _ int64, contentType string) (Object, error) {
	bucket, err := p.oss.Bucket(bucketName)
	if err != nil {
		return Object{}, err
	}
	options := []oss.Option{oss.WithContext(ctx)}
	if contentType != "" {
		options = append(options, oss.ContentType(contentType))
	}
	if err := bucket.PutObject(key, body, options...); err != nil {
		return Object{}, err
	}
	return Object{Key: key, ContentType: contentType}, nil
}
func (p *aliyunProvider) DownloadObject(ctx context.Context, bucketName, key string) (ObjectDownload, error) {
	bucket, err := p.oss.Bucket(bucketName)
	if err != nil {
		return ObjectDownload{}, err
	}
	body, err := bucket.GetObject(key, oss.WithContext(ctx))
	if err != nil {
		return ObjectDownload{}, err
	}
	return ObjectDownload{Body: body, ContentType: "application/octet-stream", Filename: key}, nil
}
func (p *aliyunProvider) DeleteObject(ctx context.Context, bucketName, key string) error {
	bucket, err := p.oss.Bucket(bucketName)
	if err != nil {
		return err
	}
	return bucket.DeleteObject(key, oss.WithContext(ctx))
}

func (p *aliyunProvider) setVersioning(ctx context.Context, name, status string) error {
	return p.oss.SetBucketVersioning(name, oss.VersioningConfig{Status: status}, oss.WithContext(ctx))
}

func bucketRegion(region, location string) string {
	return strings.TrimPrefix(firstNonEmpty(region, location), "oss-")
}
func dnsRecord(record alidns.Record) DNSRecord {
	return DNSRecord{ID: record.RecordId, RR: record.RR, Type: record.Type, Line: record.Line, Value: record.Value, TTL: record.TTL, Priority: record.Priority, Enabled: strings.EqualFold(record.Status, "enable") || strings.EqualFold(record.Status, "enabled")}
}
func applyRecordInput(request *alidns.AddDomainRecordRequest, input DNSRecordInput) {
	request.RR = input.RR
	request.Type = strings.ToUpper(input.Type)
	request.Line = recordLine(input.Line)
	request.Value = input.Value
	request.TTL = requests.NewInteger(int(input.TTL))
	request.Priority = requests.NewInteger(int(input.Priority))
}
func recordLine(line string) string {
	if strings.TrimSpace(line) == "" {
		return "default"
	}
	return strings.TrimSpace(line)
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

var _ Provider = (*aliyunProvider)(nil)
var _ = http.MethodGet
