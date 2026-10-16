package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	cloudservice "github.com/cylism/cylism-manager/internal/service/cloud"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type adoptDomainRequest struct {
	Hostname          string `json:"hostname"`
	EnvironmentID     uint   `json:"environment_id"`
	IssuerRef         string `json:"issuer_ref"`
	Description       string `json:"description"`
	CloudConnectionID uint   `json:"cloud_connection_id"`
	DNSZone           string `json:"dns_zone"`
	NodeIDs           []uint `json:"node_ids"`
}

func (h *DomainHandler) ListAdoptableCloudDomains(c *gin.Context) {
	environmentID, err := apiShared.OptionalID(strings.TrimSpace(c.Query("environment_id")))
	if err != nil || environmentID == 0 {
		apiShared.BadRequest(c, "环境 ID 必填")
		return
	}
	if _, err := h.domainEnvironment(environmentID); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if h.cloud == nil {
		apiShared.ValidationError(c, "云 DNS 未配置")
		return
	}
	connections, err := h.cloud.ListConnections()
	if err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	type candidate struct {
		Hostname          string   `json:"hostname"`
		CloudConnectionID uint     `json:"cloud_connection_id"`
		DNSZone           string   `json:"dns_zone"`
		Addresses         []string `json:"addresses"`
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
			byHost := make(map[string][]string)
			for _, record := range records {
				if !strings.EqualFold(record.Type, "A") || net.ParseIP(record.Value) == nil || net.ParseIP(record.Value).To4() == nil {
					continue
				}
				hostname := strings.ToLower(record.RR + "." + zone.Name)
				if validManagedHostname(hostname) {
					byHost[hostname] = append(byHost[hostname], record.Value)
				}
			}
			for hostname, addresses := range byHost {
				if _, err := h.network.GetManagedDomainByHostname(hostname); err == nil {
					continue
				} else if !errors.Is(err, gorm.ErrRecordNotFound) {
					apiShared.DBError(c, err.Error())
					return
				}
				result = append(result, candidate{Hostname: hostname, CloudConnectionID: connection.ID, DNSZone: zone.Name, Addresses: addresses})
			}
		}
	}
	apiShared.Success(c, result)
}

func (h *DomainHandler) AdoptCloudDomain(c *gin.Context) {
	var req adoptDomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiShared.BadRequest(c, "接管定义无效")
		return
	}
	environment, err := h.domainEnvironment(req.EnvironmentID)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	h, err = h.forEnvironment(c.Request.Context(), environment)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if h.cloud == nil {
		apiShared.ValidationError(c, "云 DNS 未配置")
		return
	}
	domain, err := domainFromRequest(domainRequest{Hostname: req.Hostname, EnvironmentID: req.EnvironmentID, IssuerRef: req.IssuerRef, Description: req.Description, Enabled: true}, nil, environment)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.validateManagedDomainPrerequisites(c.Request.Context(), domain); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	records, err := h.prepareImportDNS(c.Request.Context(), environment, domain.Hostname, req.CloudConnectionID, req.DNSZone, req.NodeIDs)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.prepareCloudDomain(c.Request.Context(), domain, environment, req.CloudConnectionID, req.DNSZone, req.NodeIDs, false); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	domain.OriginalDNSRecords = encodeDNSRecords(records)
	domain.AdoptionStatus, domain.DNSStatus = "staged", "staged"
	if err := h.network.CreateManagedDomain(domain); err != nil {
		apiShared.Conflict(c, "域名已被平台管理")
		return
	}
	assignManagedCertificateNames(domain)
	if err := h.network.UpdateManagedDomain(domain); err != nil {
		apiShared.DBError(c, err.Error())
		return
	}
	info := h.DomainInfo(c.Request.Context(), domain)
	if err := h.ensureManagedDomainCertificate(c.Request.Context(), domain); err != nil {
		info.CertificateError = err.Error()
		apiShared.SuccessWithMessage(c, info, "接管草稿已保存，原解析未变；证书申请失败，可重试")
		return
	}
	apiShared.SuccessWithMessage(c, h.DomainInfo(c.Request.Context(), domain), "接管草稿已保存，原解析未变")
}

func (h *DomainHandler) CutoverAdoptedDomain(c *gin.Context) {
	domain, ok := h.managedDomain(c)
	if !ok {
		return
	}
	if domain.AdoptionStatus != "staged" && domain.AdoptionStatus != "restored" {
		apiShared.ValidationError(c, "仅待切换或已恢复的域名可以切换解析")
		return
	}
	scoped, err := h.forDomain(c.Request.Context(), domain)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	certificate, err := scoped.k8s.GetCertificateContext(c.Request.Context(), domain.Namespace, domain.CertificateName)
	if err != nil || certificate == nil || certificate.Status != "Ready" {
		apiShared.ValidationError(c, "目标命名空间证书尚未就绪，请先完成签发")
		return
	}
	count, err := scoped.network.CountApplicationEndpointsByDomain(domain.ID)
	if err != nil || count == 0 {
		apiShared.ValidationError(c, "请先将域名绑定到目标应用入口")
		return
	}
	ready, err := scoped.k8s.ReadyIngressEndpoint(c.Request.Context(), domain.Namespace, domain.Hostname)
	if err != nil || !ready {
		apiShared.ValidationError(c, "目标 Ingress 或后端服务尚未就绪，请先确认应用 Pod 和 Service Endpoint")
		return
	}
	if err := scoped.cutoverDNS(c.Request.Context(), domain); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, scoped.DomainInfo(c.Request.Context(), domain))
}

func (h *DomainHandler) RestoreAdoptedDomain(c *gin.Context) {
	domain, ok := h.managedDomain(c)
	if !ok {
		return
	}
	if domain.AdoptionStatus != "active" && domain.AdoptionStatus != "cutover_failed" {
		apiShared.ValidationError(c, "该域名没有需要恢复的解析")
		return
	}
	if err := h.restoreDNS(c.Request.Context(), domain); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	apiShared.Success(c, h.DomainInfo(c.Request.Context(), domain))
}

func (h *DomainHandler) cutoverDNS(ctx context.Context, domain *model.ManagedDomain) error {
	records, err := h.cloudRecords(ctx, domain.CloudConnectionID, domain.DNSZone)
	if err != nil {
		return err
	}
	matching := recordsForHostname(records, domain.DNSZone, domain.Hostname)
	if hasCNAMEForHostname(records, domain.DNSZone, domain.Hostname) {
		return fmt.Errorf("域名出现 CNAME 记录，请先人工核对")
	}
	original := decodeDNSRecords(domain.OriginalDNSRecords)
	if !sameDNSRecords(matching, original) {
		return fmt.Errorf("云 DNS 已在平台外变更，请重新核对原解析")
	}
	var ids []uint
	if err := json.Unmarshal([]byte(domain.DNSNodeIDs), &ids); err != nil {
		return fmt.Errorf("读取目标节点失败: %w", err)
	}
	environment, err := h.domainEnvironment(domain.EnvironmentID)
	if err != nil {
		return err
	}
	available, err := h.dnsNodes(environment.ClusterID)
	if err != nil {
		return err
	}
	nodes, err := selectedNodes(available, ids)
	if err != nil {
		return err
	}
	target := make([]cloudservice.DNSRecord, 0, len(nodes))
	selectedAddresses := make(map[string]bool, len(nodes))
	rr := strings.TrimSuffix(domain.Hostname, "."+domain.DNSZone)
	for _, node := range nodes {
		if selectedAddresses[node.IP] {
			continue
		}
		selectedAddresses[node.IP] = true
		var existing *cloudservice.DNSRecord
		for index := range matching {
			if matching[index].Value == node.IP {
				existing = &matching[index]
				break
			}
		}
		if existing != nil {
			target = append(target, *existing)
			continue
		}
		record, err := h.cloud.CreateRecord(ctx, domain.CloudConnectionID, domain.DNSZone, cloudservice.DNSRecordInput{RR: rr, Type: "A", Value: node.IP, TTL: 600})
		if err != nil {
			return fmt.Errorf("创建目标解析失败，已创建的记录需在恢复解析中清理: %w", err)
		}
		target = append(target, record)
		domain.DNSRecords = encodeDNSRecords(target)
		domain.AdoptionStatus = "cutover_failed"
		if err := h.network.UpdateManagedDomain(domain); err != nil {
			_ = h.cloud.DeleteRecord(ctx, domain.CloudConnectionID, record.ID, true)
			return err
		}
	}
	domain.DNSRecords = encodeDNSRecords(target)
	domain.AdoptionStatus = "cutover_failed"
	if err := h.network.UpdateManagedDomain(domain); err != nil {
		return err
	}
	for _, record := range original {
		if containsRecord(target, record.ID) {
			continue
		}
		if err := h.cloud.DeleteRecord(ctx, domain.CloudConnectionID, record.ID, true); err != nil {
			return fmt.Errorf("移除原解析失败；请使用恢复解析: %w", err)
		}
	}
	domain.AdoptionStatus, domain.DNSStatus, domain.DNSError = "active", "ready", ""
	return h.network.UpdateManagedDomain(domain)
}

func (h *DomainHandler) restoreDNS(ctx context.Context, domain *model.ManagedDomain) error {
	records, err := h.cloudRecords(ctx, domain.CloudConnectionID, domain.DNSZone)
	if err != nil {
		return err
	}
	if hasCNAMEForHostname(records, domain.DNSZone, domain.Hostname) {
		return fmt.Errorf("域名出现 CNAME 记录，请先人工核对")
	}
	matching := recordsForHostname(records, domain.DNSZone, domain.Hostname)
	owned := decodeDNSRecords(domain.DNSRecords)
	original := decodeDNSRecords(domain.OriginalDNSRecords)
	for _, record := range matching {
		if saved := findDNSRecordByID(owned, record.ID); saved != nil {
			if record.Value != saved.Value || record.Line != saved.Line || record.TTL != saved.TTL {
				return fmt.Errorf("目标解析已在平台外变更，请先人工核对")
			}
			continue
		}
		if saved := findDNSRecordByID(original, record.ID); saved != nil {
			if record.Value != saved.Value || record.Line != saved.Line || record.TTL != saved.TTL {
				return fmt.Errorf("原解析已在平台外变更，请先人工核对")
			}
			continue
		}
		if !containsSavedDNSValue(original, record) {
			return fmt.Errorf("云 DNS 存在外部新增记录，请先人工核对")
		}
	}
	rr := strings.TrimSuffix(domain.Hostname, "."+domain.DNSZone)
	restored := make([]cloudservice.DNSRecord, 0, len(original))
	for _, record := range original {
		if containsDNSValue(matching, record) {
			for _, existing := range matching {
				if existing.Value == record.Value && existing.Line == record.Line {
					restored = append(restored, existing)
					break
				}
			}
			continue
		}
		ttl := record.TTL
		if ttl == 0 {
			ttl = 600
		}
		created, err := h.cloud.CreateRecord(ctx, domain.CloudConnectionID, domain.DNSZone, cloudservice.DNSRecordInput{RR: rr, Type: "A", Value: record.Value, Line: record.Line, TTL: ttl})
		if err != nil {
			return fmt.Errorf("恢复原解析失败: %w", err)
		}
		restored = append(restored, created)
	}
	domain.OriginalDNSRecords = encodeDNSRecords(restored)
	if err := h.network.UpdateManagedDomain(domain); err != nil {
		return err
	}
	for _, record := range owned {
		if containsRecord(restored, record.ID) {
			continue
		}
		if err := h.cloud.DeleteRecord(ctx, domain.CloudConnectionID, record.ID, true); err != nil {
			return fmt.Errorf("移除目标解析失败: %w", err)
		}
	}
	domain.OriginalDNSRecords = encodeDNSRecords(restored)
	domain.DNSRecords = "[]"
	domain.AdoptionStatus, domain.DNSStatus, domain.DNSError = "restored", "restored", ""
	return h.network.UpdateManagedDomain(domain)
}

func sameDNSRecords(current []cloudservice.DNSRecord, snapshot []domainDNSRecord) bool {
	if len(current) != len(snapshot) || len(snapshot) == 0 {
		return false
	}
	for _, record := range current {
		found := false
		for _, saved := range snapshot {
			if record.ID == saved.ID && record.Value == saved.Value && record.Line == saved.Line && record.TTL == saved.TTL {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func findDNSRecordByID(records []domainDNSRecord, id string) *domainDNSRecord {
	for index := range records {
		if records[index].ID == id {
			return &records[index]
		}
	}
	return nil
}

func containsDNSValue(records []cloudservice.DNSRecord, saved domainDNSRecord) bool {
	for _, record := range records {
		if record.Value == saved.Value && record.Line == saved.Line {
			return true
		}
	}
	return false
}

func containsSavedDNSValue(records []domainDNSRecord, current cloudservice.DNSRecord) bool {
	for _, record := range records {
		if record.Value == current.Value && record.Line == current.Line {
			return true
		}
	}
	return false
}
