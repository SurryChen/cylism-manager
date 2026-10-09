package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	cloudservice "github.com/cylism/cylism-manager/internal/service/cloud"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/publicsuffix"
	"gorm.io/gorm"
)

type domainCloudDNS interface {
	ListConnections() ([]model.CloudConnection, error)
	ListZones(context.Context, uint) ([]cloudservice.DNSZone, error)
	ListRecords(context.Context, uint, string, int, int) (cloudservice.DNSRecordPage, error)
	CreateRecord(context.Context, uint, string, cloudservice.DNSRecordInput) (cloudservice.DNSRecord, error)
	DeleteRecord(context.Context, uint, string, bool) error
}

type domainServerReader interface {
	ListServers() ([]model.Server, error)
}

type domainDNSRecord struct {
	ID    string `json:"id"`
	Value string `json:"value"`
}

type domainDNSNode struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	IP   string `json:"ip"`
}

func decodeDNSRecords(raw string) []domainDNSRecord {
	var records []domainDNSRecord
	_ = json.Unmarshal([]byte(raw), &records)
	return records
}

func encodeDNSRecords(records []cloudservice.DNSRecord) string {
	items := make([]domainDNSRecord, 0, len(records))
	for _, record := range records {
		items = append(items, domainDNSRecord{ID: record.ID, Value: record.Value})
	}
	data, _ := json.Marshal(items)
	return string(data)
}

func (h *DomainHandler) dnsNodes(clusterID uint) ([]domainDNSNode, error) {
	if h.servers == nil {
		return nil, fmt.Errorf("服务器列表不可用")
	}
	servers, err := h.servers.ListServers()
	if err != nil {
		return nil, err
	}
	nodes := make([]domainDNSNode, 0)
	for _, server := range servers {
		if server.ClusterID == nil || *server.ClusterID != clusterID || net.ParseIP(server.Host) == nil || net.ParseIP(server.Host).To4() == nil {
			continue
		}
		name := server.Name
		if name == "" {
			name = server.Host
		}
		nodes = append(nodes, domainDNSNode{ID: server.ID, Name: name, IP: server.Host})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes, nil
}

func selectedNodes(nodes []domainDNSNode, ids []uint) ([]domainDNSNode, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("请选择至少一个有 IPv4 地址的集群节点")
	}
	wanted := make(map[uint]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	selected := make([]domainDNSNode, 0, len(ids))
	for _, node := range nodes {
		if wanted[node.ID] {
			selected = append(selected, node)
			delete(wanted, node.ID)
		}
	}
	if len(wanted) != 0 {
		return nil, fmt.Errorf("所选节点不属于当前集群，或其连接地址不是 IPv4")
	}
	return selected, nil
}

func (h *DomainHandler) prepareCloudDomain(ctx context.Context, domain *model.ManagedDomain, environment *model.Environment, connectionID uint, zone string, nodeIDs []uint, creating bool) error {
	if h.cloud == nil || connectionID == 0 {
		return fmt.Errorf("请选择云 DNS 连接")
	}
	zone = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(zone), "."))
	if zone == "" || !validManagedHostname(zone) {
		return fmt.Errorf("请选择有效的主域名")
	}
	if creating {
		registrable, err := publicsuffix.EffectiveTLDPlusOne(zone)
		if err != nil || registrable != zone {
			return fmt.Errorf("新增域名只能选择可注册的主域名")
		}
		prefix := strings.TrimSuffix(domain.Hostname, "."+zone)
		if prefix == domain.Hostname || prefix == "" || strings.Contains(prefix, ".") {
			return fmt.Errorf("只能创建主域名下的单段三级域名")
		}
	} else if !strings.HasSuffix(domain.Hostname, "."+zone) {
		return fmt.Errorf("域名不属于所选主域名")
	}
	connections, err := h.cloud.ListConnections()
	if err != nil {
		return err
	}
	found := false
	for _, connection := range connections {
		if connection.ID == connectionID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("云 DNS 连接不存在")
	}
	zones, err := h.cloud.ListZones(ctx, connectionID)
	if err != nil {
		return fmt.Errorf("读取云域名失败: %w", err)
	}
	found = false
	for _, item := range zones {
		if strings.EqualFold(item.Name, zone) {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("所选主域名不属于云 DNS 连接")
	}
	nodes, err := h.dnsNodes(environment.ClusterID)
	if err != nil {
		return err
	}
	if _, err := selectedNodes(nodes, nodeIDs); err != nil {
		return err
	}
	encoded, _ := json.Marshal(nodeIDs)
	domain.CloudConnectionID, domain.DNSZone, domain.DNSNodeIDs = connectionID, zone, string(encoded)
	domain.DNSStatus = "pending"
	return nil
}

func (h *DomainHandler) DNSOptions(c *gin.Context) {
	environmentID, err := apiShared.OptionalID(strings.TrimSpace(c.Query("environment_id")))
	if err != nil || environmentID == 0 {
		apiShared.BadRequest(c, "环境 ID 必填")
		return
	}
	environment, err := h.domainEnvironment(environmentID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	nodes, err := h.dnsNodes(environment.ClusterID)
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	connections, err := h.cloud.ListConnections()
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	type connectionOption struct {
		ID    uint                   `json:"id"`
		Name  string                 `json:"name"`
		Zones []cloudservice.DNSZone `json:"zones"`
	}
	options := make([]connectionOption, 0)
	for _, connection := range connections {
		zones, err := h.cloud.ListZones(c.Request.Context(), connection.ID)
		if err == nil && len(zones) > 0 {
			options = append(options, connectionOption{ID: connection.ID, Name: connection.Name, Zones: zones})
		}
	}
	apiShared.Success(c, gin.H{"connections": options, "nodes": nodes})
}

func (h *DomainHandler) cloudRecords(ctx context.Context, connectionID uint, zone string) ([]cloudservice.DNSRecord, error) {
	all := make([]cloudservice.DNSRecord, 0)
	for page := 1; ; page++ {
		result, err := h.cloud.ListRecords(ctx, connectionID, zone, page, 100)
		if err != nil {
			return nil, err
		}
		all = append(all, result.Records...)
		if len(result.Records) < 100 || int64(page*100) >= result.Total {
			return all, nil
		}
	}
}

func recordsForHostname(records []cloudservice.DNSRecord, zone, hostname string) []cloudservice.DNSRecord {
	rr := strings.TrimSuffix(hostname, "."+zone)
	matching := make([]cloudservice.DNSRecord, 0)
	for _, record := range records {
		if strings.EqualFold(record.RR, rr) && strings.EqualFold(record.Type, "A") {
			matching = append(matching, record)
		}
	}
	return matching
}

func (h *DomainHandler) checkAvailableCloudRecord(ctx context.Context, domain *model.ManagedDomain) error {
	records, err := h.cloudRecords(ctx, domain.CloudConnectionID, domain.DNSZone)
	if err != nil {
		return fmt.Errorf("检查云 DNS 记录失败: %w", err)
	}
	rr := strings.TrimSuffix(domain.Hostname, "."+domain.DNSZone)
	for _, record := range records {
		if strings.EqualFold(record.RR, rr) && (strings.EqualFold(record.Type, "A") || strings.EqualFold(record.Type, "CNAME")) {
			return fmt.Errorf("云 DNS 中已有该域名的解析记录，请使用导入流程")
		}
	}
	return nil
}

func (h *DomainHandler) prepareImportDNS(ctx context.Context, environment *model.Environment, hostname string, connectionID uint, zone string, nodeIDs []uint) ([]cloudservice.DNSRecord, error) {
	temporary := &model.ManagedDomain{Hostname: hostname}
	if err := h.prepareCloudDomain(ctx, temporary, environment, connectionID, zone, nodeIDs, false); err != nil {
		return nil, err
	}
	records, err := h.cloudRecords(ctx, connectionID, temporary.DNSZone)
	if err != nil {
		return nil, fmt.Errorf("读取云 DNS 记录失败: %w", err)
	}
	matching := recordsForHostname(records, temporary.DNSZone, hostname)
	if len(matching) == 0 {
		return nil, fmt.Errorf("云 DNS 中不存在对应的 A 记录")
	}
	return matching, nil
}

func (h *DomainHandler) ListImportableCloudDomains(c *gin.Context) {
	environmentID, err := apiShared.OptionalID(strings.TrimSpace(c.Query("environment_id")))
	if err != nil || environmentID == 0 {
		apiShared.BadRequest(c, "环境 ID 必填")
		return
	}
	environment, err := h.domainEnvironment(environmentID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	scoped, err := h.forEnvironment(c.Request.Context(), environment)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	certificates, err := scoped.network.ListCertificatesContext(c.Request.Context())
	if err != nil {
		apiShared.K8sAPIError(c, err.Error())
		return
	}
	connections, err := h.cloud.ListConnections()
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	type candidate struct {
		Certificate  k8s.CertInfo `json:"certificate"`
		ConnectionID uint         `json:"cloud_connection_id"`
		Zone         string       `json:"dns_zone"`
		Addresses    []string     `json:"addresses"`
	}
	result := make([]candidate, 0)
	for _, connection := range connections {
		zones, err := h.cloud.ListZones(c.Request.Context(), connection.ID)
		if err != nil {
			continue
		}
		for _, zone := range zones {
			records, err := h.cloudRecords(c.Request.Context(), connection.ID, zone.Name)
			if err != nil {
				continue
			}
			for _, certificate := range certificates {
				if certificate.Namespace != environment.Namespace || !importableCertificate(certificate) {
					continue
				}
				hostname := strings.ToLower(certificate.Domains[0])
				existing, lookupErr := h.network.GetManagedDomainByHostname(hostname)
				if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
					apiShared.DBError(c, lookupErr.Error())
					return
				}
				if errors.Is(lookupErr, gorm.ErrRecordNotFound) {
					existing = nil
				}
				if existing != nil && (existing.EnvironmentID != 0 || existing.Namespace != environment.Namespace) {
					continue
				}
				if !strings.HasSuffix(hostname, "."+strings.ToLower(zone.Name)) {
					continue
				}
				matching := recordsForHostname(records, zone.Name, hostname)
				if len(matching) == 0 {
					continue
				}
				addresses := make([]string, 0, len(matching))
				for _, record := range matching {
					addresses = append(addresses, record.Value)
				}
				result = append(result, candidate{Certificate: certificate, ConnectionID: connection.ID, Zone: zone.Name, Addresses: addresses})
			}
		}
	}
	apiShared.Success(c, result)
}

func (h *DomainHandler) syncDNS(ctx context.Context, domain *model.ManagedDomain) error {
	var ids []uint
	_ = json.Unmarshal([]byte(domain.DNSNodeIDs), &ids)
	environment, err := h.domainEnvironment(domain.EnvironmentID)
	if err != nil {
		return h.failDNS(domain, err)
	}
	nodes, err := h.dnsNodes(environment.ClusterID)
	if err != nil {
		return h.failDNS(domain, err)
	}
	selected, err := selectedNodes(nodes, ids)
	if err != nil {
		return h.failDNS(domain, err)
	}
	records, err := h.cloudRecords(ctx, domain.CloudConnectionID, domain.DNSZone)
	if err != nil {
		return h.failDNS(domain, err)
	}
	rr := strings.TrimSuffix(domain.Hostname, "."+domain.DNSZone)
	matching := recordsForHostname(records, domain.DNSZone, domain.Hostname)
	if len(decodeDNSRecords(domain.DNSRecords)) == 0 && len(matching) > 0 {
		return h.failDNS(domain, fmt.Errorf("云 DNS 中已有该域名的 A 记录，请先从导入流程接管"))
	}
	for _, record := range records {
		if strings.EqualFold(record.RR, rr) && strings.EqualFold(record.Type, "CNAME") {
			return h.failDNS(domain, fmt.Errorf("云 DNS 中已有该域名的 CNAME 记录"))
		}
	}
	wanted := make(map[string]bool)
	for _, node := range selected {
		wanted[node.IP] = true
	}
	kept := make([]cloudservice.DNSRecord, 0)
	for _, record := range matching {
		if wanted[record.Value] {
			kept = append(kept, record)
			delete(wanted, record.Value)
		}
	}
	for address := range wanted {
		created, err := h.cloud.CreateRecord(ctx, domain.CloudConnectionID, domain.DNSZone, cloudservice.DNSRecordInput{RR: rr, Type: "A", Value: address, TTL: 600})
		if err != nil {
			domain.DNSRecords = encodeDNSRecords(kept)
			return h.failDNS(domain, err)
		}
		kept = append(kept, created)
		domain.DNSRecords = encodeDNSRecords(kept)
		if err := h.network.UpdateManagedDomain(domain); err != nil {
			_ = h.cloud.DeleteRecord(ctx, domain.CloudConnectionID, created.ID, true)
			return fmt.Errorf("保存 DNS 记录引用失败: %w", err)
		}
	}
	owned := make(map[string]bool)
	for _, record := range decodeDNSRecords(domain.DNSRecords) {
		owned[record.ID] = true
	}
	for _, record := range matching {
		if owned[record.ID] && !containsRecord(kept, record.ID) {
			if err := h.cloud.DeleteRecord(ctx, domain.CloudConnectionID, record.ID, true); err != nil {
				return h.failDNS(domain, err)
			}
		}
	}
	domain.DNSRecords = encodeDNSRecords(kept)
	domain.DNSStatus, domain.DNSError = "ready", ""
	return h.network.UpdateManagedDomain(domain)
}

func containsRecord(records []cloudservice.DNSRecord, id string) bool {
	for _, record := range records {
		if record.ID == id {
			return true
		}
	}
	return false
}

func (h *DomainHandler) failDNS(domain *model.ManagedDomain, err error) error {
	domain.DNSStatus, domain.DNSError = "failed", err.Error()
	if saveErr := h.network.UpdateManagedDomain(domain); saveErr != nil {
		return fmt.Errorf("%w; 保存 DNS 状态失败: %v", err, saveErr)
	}
	return err
}

func (h *DomainHandler) RetryDNS(c *gin.Context) {
	domain, ok := h.managedDomain(c)
	if !ok {
		return
	}
	if domain.CloudConnectionID == 0 {
		apiShared.ValidationError(c, "域名尚未绑定云 DNS")
		return
	}
	if err := h.syncDNS(c.Request.Context(), domain); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if !isImportedDomain(domain) {
		scoped, err := h.forDomain(c.Request.Context(), domain)
		if err != nil || scoped.ensureManagedDomainCertificate(c.Request.Context(), domain) != nil {
			apiShared.SuccessWithMessage(c, h.DomainInfo(c.Request.Context(), domain), "DNS 已同步；证书仍需在列表中重试签发")
			return
		}
	}
	apiShared.SuccessWithMessage(c, h.DomainInfo(c.Request.Context(), domain), "DNS 解析已同步")
}
