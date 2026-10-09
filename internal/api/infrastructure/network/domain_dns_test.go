package network

import (
	"context"
	"fmt"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	cloudservice "github.com/cylism/cylism-manager/internal/service/cloud"
	networkservice "github.com/cylism/cylism-manager/internal/service/network"
	"github.com/cylism/cylism-manager/internal/store"
)

type fakeDomainCloud struct {
	records []cloudservice.DNSRecord
	creates int
}

func (f *fakeDomainCloud) ListConnections() ([]model.CloudConnection, error) {
	return []model.CloudConnection{{ID: 3, Name: "primary"}}, nil
}
func (f *fakeDomainCloud) ListZones(context.Context, uint) ([]cloudservice.DNSZone, error) {
	return []cloudservice.DNSZone{{Name: "example.com"}}, nil
}
func (f *fakeDomainCloud) ListRecords(context.Context, uint, string, int, int) (cloudservice.DNSRecordPage, error) {
	return cloudservice.DNSRecordPage{Records: append([]cloudservice.DNSRecord(nil), f.records...), Total: int64(len(f.records))}, nil
}
func (f *fakeDomainCloud) CreateRecord(_ context.Context, _ uint, _ string, input cloudservice.DNSRecordInput) (cloudservice.DNSRecord, error) {
	f.creates++
	record := cloudservice.DNSRecord{ID: fmt.Sprint(f.creates), RR: input.RR, Type: input.Type, Value: input.Value}
	f.records = append(f.records, record)
	return record, nil
}
func (f *fakeDomainCloud) DeleteRecord(_ context.Context, _ uint, id string, _ bool) error {
	for index, record := range f.records {
		if record.ID == id {
			f.records = append(f.records[:index], f.records[index+1:]...)
			break
		}
	}
	return nil
}

type fakeDomainServers []model.Server

func (f fakeDomainServers) ListServers() ([]model.Server, error) { return f, nil }

func TestCloudDomainValidatesZoneAndClusterNodes(t *testing.T) {
	clusterID := uint(7)
	cloud := &fakeDomainCloud{}
	h := NewDomainHandlerWithDependencies(nil, nil).WithCloudDNS(nil, fakeDomainServers{{ID: 2, Host: "203.0.113.12", ClusterID: &clusterID}})
	h.cloud = cloud
	environment := &model.Environment{ID: 1, ClusterID: clusterID}
	domain := &model.ManagedDomain{Hostname: "api.example.com"}
	if err := h.prepareCloudDomain(context.Background(), domain, environment, 3, "example.com", []uint{2}, true); err != nil {
		t.Fatal(err)
	}
	if domain.CloudConnectionID != 3 || domain.DNSZone != "example.com" {
		t.Fatalf("unexpected DNS binding: %#v", domain)
	}
	if err := h.prepareCloudDomain(context.Background(), &model.ManagedDomain{Hostname: "a.b.example.com"}, environment, 3, "example.com", []uint{2}, true); err == nil {
		t.Fatal("expected multi-label prefix to be rejected")
	}
	if err := h.prepareCloudDomain(context.Background(), &model.ManagedDomain{Hostname: "api.sub.example.com"}, environment, 3, "sub.example.com", []uint{2}, true); err == nil {
		t.Fatal("expected delegated subzone to be rejected for creation")
	}
	if err := h.prepareCloudDomain(context.Background(), domain, environment, 3, "example.com", []uint{99}, true); err == nil {
		t.Fatal("expected foreign node to be rejected")
	}
}

func TestCloudDomainSyncIsIdempotentAndRejectsExistingUnmanagedRecord(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	clusterID := uint(7)
	environment := &model.Environment{ProjectID: 1, Name: "prod", Namespace: "app-prod", ClusterID: clusterID}
	if err := st.CreateEnvironment(environment); err != nil {
		t.Fatal(err)
	}
	cloud := &fakeDomainCloud{}
	h := NewDomainHandlerWithDependencies(nil, networkservice.NewService(st, st)).WithCloudDNS(nil, fakeDomainServers{{ID: 2, Host: "203.0.113.12", ClusterID: &clusterID}})
	h.cloud = cloud
	domain := &model.ManagedDomain{Hostname: "api.example.com", EnvironmentID: environment.ID}
	if err := h.prepareCloudDomain(context.Background(), domain, environment, 3, "example.com", []uint{2}, true); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateManagedDomain(domain); err != nil {
		t.Fatal(err)
	}
	if err := h.syncDNS(context.Background(), domain); err != nil {
		t.Fatal(err)
	}
	if err := h.syncDNS(context.Background(), domain); err != nil {
		t.Fatal(err)
	}
	if cloud.creates != 1 || domain.DNSStatus != "ready" || len(decodeDNSRecords(domain.DNSRecords)) != 1 {
		t.Fatalf("unexpected DNS result: creates=%d, domain=%#v", cloud.creates, domain)
	}
	other := &model.ManagedDomain{Hostname: "web.example.com", CloudConnectionID: 3, DNSZone: "example.com"}
	cloud.records = append(cloud.records, cloudservice.DNSRecord{ID: "external", RR: "web", Type: "A", Value: "203.0.113.33"})
	if err := h.checkAvailableCloudRecord(context.Background(), other); err == nil {
		t.Fatal("expected existing cloud record to block new domain")
	}
}

func TestImportRequiresMatchingCloudARecord(t *testing.T) {
	clusterID := uint(7)
	cloud := &fakeDomainCloud{}
	h := NewDomainHandlerWithDependencies(nil, nil).WithCloudDNS(nil, fakeDomainServers{{ID: 2, Host: "203.0.113.12", ClusterID: &clusterID}})
	h.cloud = cloud
	environment := &model.Environment{ID: 1, ClusterID: clusterID}
	if _, err := h.prepareImportDNS(context.Background(), environment, "api.example.com", 3, "example.com", []uint{2}); err == nil {
		t.Fatal("expected missing cloud A record to block import")
	}
	cloud.records = []cloudservice.DNSRecord{{ID: "existing", RR: "api", Type: "A", Value: "203.0.113.33"}}
	records, err := h.prepareImportDNS(context.Background(), environment, "api.example.com", 3, "example.com", []uint{2})
	if err != nil || len(records) != 1 || records[0].ID != "existing" {
		t.Fatalf("unexpected importable records: %#v, err=%v", records, err)
	}
}
