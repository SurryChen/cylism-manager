package k8s

import (
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var certGVR = schema.GroupVersionResource{
	Group:    "cert-manager.io",
	Version:  "v1",
	Resource: "certificates",
}

var issuerGVR = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "issuers"}
var clusterIssuerGVR = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "clusterissuers"}
var certificateRequestGVR = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificaterequests"}
var orderGVR = schema.GroupVersionResource{Group: "acme.cert-manager.io", Version: "v1", Resource: "orders"}
var challengeGVR = schema.GroupVersionResource{Group: "acme.cert-manager.io", Version: "v1", Resource: "challenges"}

// CertInfo 证书展示信息
type CertInfo struct {
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace"`
	Domains     []string `json:"domains"`
	Status      string   `json:"status"` // Ready / Issuing / Failed
	ExpiryDate  string   `json:"expiry_date"`
	CreatedAt   string   `json:"created_at"`
	Issuer      string   `json:"issuer"`
	IssuerKind  string   `json:"issuer_kind"`
	SecretName  string   `json:"secret_name"`
	RenewalTime string   `json:"renewal_time"`
	Reason      string   `json:"reason,omitempty"`
}

type IssuerInfo struct {
	Name                 string `json:"name"`
	Namespace            string `json:"namespace,omitempty"`
	Kind                 string `json:"kind"`
	Ready                bool   `json:"ready"`
	Reason               string `json:"reason,omitempty"`
	Mode                 string `json:"mode,omitempty"`
	Email                string `json:"email,omitempty"`
	DNSProvider          string `json:"dns_provider,omitempty"`
	CredentialSecretName string `json:"credential_secret_name,omitempty"`
}

// IssuerRequest describes one intentionally supported cert-manager issuer mode.
type IssuerRequest struct {
	Name                 string `json:"name"`
	Namespace            string `json:"namespace"`
	Kind                 string `json:"kind"`
	Mode                 string `json:"mode"`
	Email                string `json:"email"`
	Server               string `json:"server"`
	IngressClass         string `json:"ingress_class"`
	DNSProvider          string `json:"dns_provider"`
	CredentialSecretName string `json:"-"`
}

// CertificateOperation captures cert-manager resources belonging to one Certificate.
type CertificateOperation struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Reason    string `json:"reason,omitempty"`
	Domain    string `json:"domain,omitempty"`
	Type      string `json:"type,omitempty"`
	CreatedAt string `json:"created_at"`
}
type CreateCertificateRequest struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Domains    []string `json:"domains"`
	IssuerRef  string   `json:"issuer_ref"`
	IssuerKind string   `json:"issuer_kind"`
	SecretName string   `json:"secret_name,omitempty"`
}

// ListCertificates 列出所有 Certificate
func (c *Client) ListCertificates() ([]CertInfo, error) {
	dynamicClient, err := c.dynamicClient()
	if err != nil {
		return nil, err
	}

	list, err := dynamicClient.Resource(certGVR).Namespace("").List(c.Ctx(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list certificates: %w", err)
	}

	var result []CertInfo
	for _, item := range list.Items {
		info := certToInfo(&item)
		result = append(result, info)
	}
	return result, nil
}

func (c *Client) ListIssuers() ([]IssuerInfo, error) {
	dynamicClient, err := c.dynamicClient()
	if err != nil {
		return nil, err
	}
	result := []IssuerInfo{}
	for _, source := range []struct {
		gvr        schema.GroupVersionResource
		kind       string
		namespaced bool
	}{{issuerGVR, "Issuer", true}, {clusterIssuerGVR, "ClusterIssuer", false}} {
		resource := dynamicClient.Resource(source.gvr)
		var list *unstructured.UnstructuredList
		if source.namespaced {
			list, err = resource.Namespace("").List(c.Ctx(), metav1.ListOptions{})
		} else {
			list, err = resource.List(c.Ctx(), metav1.ListOptions{})
		}
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", source.kind, err)
		}
		for i := range list.Items {
			result = append(result, issuerToInfo(&list.Items[i], source.kind))
		}
	}
	return result, nil
}

func issuerToInfo(item *unstructured.Unstructured, kind string) IssuerInfo {
	info := IssuerInfo{Name: item.GetName(), Namespace: item.GetNamespace(), Kind: kind}
	if _, ok, _ := unstructured.NestedMap(item.Object, "spec", "selfSigned"); ok {
		info.Mode = "self_signed"
	} else if solvers, ok, _ := unstructured.NestedSlice(item.Object, "spec", "acme", "solvers"); ok {
		info.Mode = "acme_http01"
		for _, solver := range solvers {
			if value, ok := solver.(map[string]interface{}); ok {
				if dns01, exists := value["dns01"].(map[string]interface{}); exists {
					info.Mode = "acme_dns01"
					solverName, _, _ := nestedStringMap(dns01, "webhook", "solverName")
					for providerID, provider := range dnsProviders {
						if solverName == provider.Info().ID || (solverName == "alidns" && providerID == "alidns") {
							info.DNSProvider = providerID
							info.CredentialSecretName = provider.CredentialSecretName(value)
							break
						}
					}
					break
				}
			}
		}
	}
	info.Email, _, _ = unstructured.NestedString(item.Object, "spec", "acme", "email")
	if conditions, ok, _ := unstructured.NestedSlice(item.Object, "status", "conditions"); ok {
		for _, raw := range conditions {
			if condition, ok := raw.(map[string]interface{}); ok && condition["type"] == "Ready" {
				info.Ready = condition["status"] == "True"
				info.Reason, _, _ = unstructured.NestedString(condition, "reason")
			}
		}
	}
	return info
}

// CreateIssuer creates only the explicitly supported Issuer shapes.
func (c *Client) CreateIssuer(request IssuerRequest) (*IssuerInfo, error) {
	return c.applyIssuer(request, false)
}

// UpdateIssuer replaces a managed Issuer definition while retaining its cert-manager status.
func (c *Client) UpdateIssuer(request IssuerRequest) (*IssuerInfo, error) {
	return c.applyIssuer(request, true)
}

func (c *Client) applyIssuer(request IssuerRequest, update bool) (*IssuerInfo, error) {
	dynamicClient, err := c.dynamicClient()
	if err != nil {
		return nil, err
	}
	request.Name = strings.TrimSpace(request.Name)
	request.Namespace = strings.TrimSpace(request.Namespace)
	request.Kind = strings.TrimSpace(request.Kind)
	request.Mode = strings.TrimSpace(request.Mode)
	if request.Kind != "Issuer" && request.Kind != "ClusterIssuer" {
		return nil, fmt.Errorf("Issuer 类型必须为 Issuer 或 ClusterIssuer")
	}
	if request.Name == "" || (request.Kind == "Issuer" && request.Namespace == "") {
		return nil, fmt.Errorf("签发者名称和命名空间不能为空")
	}
	object, err := issuerObject(request)
	if err != nil {
		return nil, err
	}
	var applied *unstructured.Unstructured
	if request.Kind == "Issuer" {
		resource := dynamicClient.Resource(issuerGVR).Namespace(request.Namespace)
		if update {
			applied, err = resource.Update(c.Ctx(), object, metav1.UpdateOptions{})
		} else {
			applied, err = resource.Create(c.Ctx(), object, metav1.CreateOptions{})
		}
	} else {
		resource := dynamicClient.Resource(clusterIssuerGVR)
		if update {
			applied, err = resource.Update(c.Ctx(), object, metav1.UpdateOptions{})
		} else {
			applied, err = resource.Create(c.Ctx(), object, metav1.CreateOptions{})
		}
	}
	if err != nil {
		return nil, err
	}
	info := issuerToInfo(applied, request.Kind)
	return &info, nil
}

func issuerObject(request IssuerRequest) (*unstructured.Unstructured, error) {
	metadata := map[string]interface{}{"name": request.Name}
	if request.Kind == "Issuer" {
		metadata["namespace"] = request.Namespace
	}
	spec := map[string]interface{}{}
	switch request.Mode {
	case "self_signed":
		spec["selfSigned"] = map[string]interface{}{}
	case "acme_http01", "acme_dns01":
		if strings.TrimSpace(request.Email) == "" {
			return nil, fmt.Errorf("ACME 签发者必须填写联系邮箱")
		}
		server := strings.TrimSpace(request.Server)
		if server == "" {
			server = "https://acme-v02.api.letsencrypt.org/directory"
		}
		acme := map[string]interface{}{"email": strings.TrimSpace(request.Email), "server": server, "privateKeySecretRef": map[string]interface{}{"name": request.Name + "-acme-account"}}
		if request.Mode == "acme_http01" {
			ingressClass := strings.TrimSpace(request.IngressClass)
			if ingressClass == "" {
				ingressClass = "traefik"
			}
			acme["solvers"] = []interface{}{map[string]interface{}{"http01": map[string]interface{}{"ingress": map[string]interface{}{"ingressClassName": ingressClass}}}}
		} else {
			provider, ok := GetDNSProvider(request.DNSProvider)
			if !ok {
				return nil, fmt.Errorf("不支持的 DNS Provider: %s", request.DNSProvider)
			}
			if strings.TrimSpace(request.CredentialSecretName) == "" {
				return nil, fmt.Errorf("DNS-01 签发者必须选择凭据")
			}
			acme["solvers"] = []interface{}{provider.Solver(request.CredentialSecretName)}
		}
		spec["acme"] = acme
	default:
		return nil, fmt.Errorf("签发模式仅支持自签名、ACME HTTP-01 或 DNS-01")
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "cert-manager.io/v1", "kind": request.Kind, "metadata": metadata, "spec": spec}}, nil
}

func (c *Client) DeleteIssuer(kind, namespace, name string) error {
	dynamicClient, err := c.dynamicClient()
	if err != nil {
		return err
	}
	if kind == "ClusterIssuer" {
		return dynamicClient.Resource(clusterIssuerGVR).Delete(c.Ctx(), name, metav1.DeleteOptions{})
	}
	if kind != "Issuer" || namespace == "" {
		return fmt.Errorf("Issuer 类型或命名空间无效")
	}
	return dynamicClient.Resource(issuerGVR).Namespace(namespace).Delete(c.Ctx(), name, metav1.DeleteOptions{})
}

// ListCertificateOperations follows owner references from Certificate to request, order and challenge resources.
func (c *Client) ListCertificateOperations(namespace, certificateName string) ([]CertificateOperation, error) {
	dynamicClient, err := c.dynamicClient()
	if err != nil {
		return nil, err
	}
	requests, err := dynamicClient.Resource(certificateRequestGVR).Namespace(namespace).List(c.Ctx(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list certificaterequests: %w", err)
	}
	requestNames := map[string]bool{}
	operations := []CertificateOperation{}
	for i := range requests.Items {
		item := &requests.Items[i]
		if ownedBy(item, "Certificate", certificateName) {
			requestNames[item.GetName()] = true
			operations = append(operations, operationToInfo(item, "CertificateRequest"))
		}
	}
	orders, err := dynamicClient.Resource(orderGVR).Namespace(namespace).List(c.Ctx(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	orderNames := map[string]bool{}
	for i := range orders.Items {
		item := &orders.Items[i]
		if ownedByAny(item, "CertificateRequest", requestNames) {
			orderNames[item.GetName()] = true
			operations = append(operations, operationToInfo(item, "Order"))
		}
	}
	challenges, err := dynamicClient.Resource(challengeGVR).Namespace(namespace).List(c.Ctx(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list challenges: %w", err)
	}
	for i := range challenges.Items {
		item := &challenges.Items[i]
		if ownedByAny(item, "Order", orderNames) {
			operations = append(operations, operationToInfo(item, "Challenge"))
		}
	}
	return operations, nil
}

func ownedBy(item *unstructured.Unstructured, kind, name string) bool {
	return ownedByAny(item, kind, map[string]bool{name: true})
}
func ownedByAny(item *unstructured.Unstructured, kind string, names map[string]bool) bool {
	for _, ref := range item.GetOwnerReferences() {
		if ref.Kind == kind && names[ref.Name] {
			return true
		}
	}
	return false
}
func operationToInfo(item *unstructured.Unstructured, kind string) CertificateOperation {
	info := CertificateOperation{Kind: kind, Name: item.GetName(), Namespace: item.GetNamespace(), CreatedAt: item.GetCreationTimestamp().Format("2006-01-02 15:04"), Status: "Pending"}
	info.Reason, _, _ = unstructured.NestedString(item.Object, "status", "reason")
	if state, ok, _ := unstructured.NestedString(item.Object, "status", "state"); ok && state != "" {
		info.Status = state
	}
	if conditions, ok, _ := unstructured.NestedSlice(item.Object, "status", "conditions"); ok {
		for _, raw := range conditions {
			condition, valid := raw.(map[string]interface{})
			if !valid || condition["type"] != "Ready" {
				continue
			}
			if condition["status"] == "True" {
				info.Status = "Ready"
			} else if condition["status"] == "False" {
				info.Status = "Failed"
			}
			if reason, valid := condition["reason"].(string); valid {
				info.Reason = reason
			}
		}
	}
	if kind == "Challenge" {
		info.Domain, _, _ = unstructured.NestedString(item.Object, "spec", "dnsName")
		info.Type, _, _ = unstructured.NestedString(item.Object, "spec", "type")
	}
	return info
}

func (c *Client) CreateCertificate(request CreateCertificateRequest) (*CertInfo, error) {
	dynamicClient, err := c.dynamicClient()
	if err != nil {
		return nil, err
	}
	object := certificateObject(request)
	created, err := dynamicClient.Resource(certGVR).Namespace(request.Namespace).Create(c.Ctx(), object, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	info := certToInfo(created)
	return &info, nil
}

// EnsureCertificate applies the controlled Certificate shape used by a managed domain.
func (c *Client) EnsureCertificate(request CreateCertificateRequest) (*CertInfo, error) {
	dynamicClient, err := c.dynamicClient()
	if err != nil {
		return nil, err
	}
	object := certificateObject(request)
	resource := dynamicClient.Resource(certGVR).Namespace(request.Namespace)
	existing, err := resource.Get(c.Ctx(), request.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		created, createErr := resource.Create(c.Ctx(), object, metav1.CreateOptions{})
		if createErr != nil {
			return nil, createErr
		}
		info := certToInfo(created)
		return &info, nil
	}
	if err != nil {
		return nil, err
	}
	object.SetResourceVersion(existing.GetResourceVersion())
	updated, err := resource.Update(c.Ctx(), object, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	info := certToInfo(updated)
	return &info, nil
}

func (c *Client) GetCertificate(namespace, name string) (*CertInfo, error) {
	dynamicClient, err := c.dynamicClient()
	if err != nil {
		return nil, err
	}
	certificate, err := dynamicClient.Resource(certGVR).Namespace(namespace).Get(c.Ctx(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	info := certToInfo(certificate)
	return &info, nil
}

func certificateObject(request CreateCertificateRequest) *unstructured.Unstructured {
	issuerKind := request.IssuerKind
	if issuerKind == "" {
		issuerKind = "ClusterIssuer"
	}
	secretName := request.SecretName
	if secretName == "" {
		secretName = request.Name + "-tls"
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "cert-manager.io/v1", "kind": "Certificate", "metadata": map[string]interface{}{"name": request.Name, "namespace": request.Namespace}, "spec": map[string]interface{}{"secretName": secretName, "dnsNames": stringSlice(request.Domains), "issuerRef": map[string]interface{}{"name": request.IssuerRef, "kind": issuerKind}}}}
}

func stringSlice(values []string) []interface{} {
	result := make([]interface{}, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

// DeleteCertificate 删除 Certificate
func (c *Client) DeleteCertificate(namespace, name string) error {
	dynamicClient, err := c.dynamicClient()
	if err != nil {
		return err
	}
	return dynamicClient.Resource(certGVR).Namespace(namespace).Delete(c.Ctx(), name, metav1.DeleteOptions{})
}

func certToInfo(item *unstructured.Unstructured) CertInfo {
	info := CertInfo{
		Name:      item.GetName(),
		Namespace: item.GetNamespace(),
		CreatedAt: item.GetCreationTimestamp().Format("2006-01-02 15:04"),
	}

	// DNS names
	if spec, ok := item.Object["spec"].(map[string]interface{}); ok {
		if dnsNames, ok := spec["dnsNames"].([]interface{}); ok {
			for _, d := range dnsNames {
				info.Domains = append(info.Domains, fmt.Sprintf("%v", d))
			}
		}
		info.Issuer, _, _ = unstructured.NestedString(spec, "issuerRef", "name")
		info.IssuerKind, _, _ = unstructured.NestedString(spec, "issuerRef", "kind")
		info.SecretName, _, _ = unstructured.NestedString(spec, "secretName")
	}

	// A Ready=False condition is not necessarily a terminal failure. cert-manager
	// reports transient reasons such as DoesNotExist while it creates the Secret.
	if status, ok := item.Object["status"].(map[string]interface{}); ok {
		if conditions, ok := status["conditions"].([]interface{}); ok {
			for _, c := range conditions {
				if cond, ok := c.(map[string]interface{}); ok {
					if cond["type"] == "Ready" && cond["status"] == "True" {
						info.Status = "Ready"
					}
				}
			}
		}
		info.Status = "Issuing"
		if conditions, ok := status["conditions"].([]interface{}); ok {
			for _, raw := range conditions {
				if cond, ok := raw.(map[string]interface{}); ok && cond["type"] == "Ready" {
					if reason, ok := cond["reason"].(string); ok {
						info.Reason = reason
					}
					if cond["status"] == "False" && certificateFailureReason(info.Reason) {
						info.Status = "Failed"
					}
				}
			}
		}
		if notAfter, ok := status["notAfter"].(string); ok {
			info.ExpiryDate = notAfter
		}
		if renewalTime, ok := status["renewalTime"].(string); ok {
			info.RenewalTime = renewalTime
		}
	}

	return info
}

func certificateFailureReason(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "failed", "denied", "invalidrequest", "requestfailed":
		return true
	default:
		return false
	}
}
