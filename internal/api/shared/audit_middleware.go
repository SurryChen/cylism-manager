package shared

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	security "github.com/cylism/cylism-manager/internal/security"
	auditservice "github.com/cylism/cylism-manager/internal/service/audit"
	"github.com/gin-gonic/gin"
)

const auditRequestIDKey = "audit_request_id"

// AuditMiddleware establishes request correlation and writes an event only
// for routes from the explicit high-risk resource catalog. Handler-specific
// events take precedence for operations that need a richer target or detail.
func AuditMiddleware(logs repository.AuditRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := EnsureRequestID(c)
		c.Next()
		if logs == nil {
			return
		}
		action, resourceType, ok := catalogedAuditAction(c.Request.Method, c.FullPath())
		if !ok {
			return
		}
		outcome := model.AuditOutcomeSucceeded
		if c.Writer.Status() >= http.StatusBadRequest {
			outcome = model.AuditOutcomeFailed
			if c.Writer.Status() == http.StatusUnauthorized || c.Writer.Status() == http.StatusForbidden {
				outcome = model.AuditOutcomeDenied
			}
		}
		source := model.AuditSourceAPI
		if _, delegated := c.Get("delegation"); delegated {
			source = "delegation"
		}
		_ = auditservice.NewService(logs).Record(auditservice.AuditEventInput{
			Action: action, ResourceType: resourceType, ResourceID: extractRouteResourceID(c),
			Actor: ActorFromContext(c), Source: source, Outcome: outcome,
			TargetName: extractRouteTargetName(c, resourceType), Summary: model.AuditSummaryFallback(action, resourceType, extractRouteResourceID(c)), RequestID: requestID,
			Metadata: map[string]any{"route": c.FullPath(), "method": c.Request.Method},
		})
	}
}

// EnsureRequestID makes an audit correlation ID available to handlers that
// deliberately reject a request before the general audit middleware runs.
func EnsureRequestID(c *gin.Context) string {
	if requestID := RequestID(c); requestID != "" {
		return requestID
	}
	requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
	if requestID == "" {
		requestID = newAuditRequestID()
	}
	c.Set(auditRequestIDKey, requestID)
	c.Header("X-Request-ID", requestID)
	return requestID
}

func catalogedAuditAction(method, path string) (string, string, bool) {
	// These paths write their own richer semantic event.
	if strings.Contains(path, "/node-registry-mirrors") || strings.Contains(path, "/agent-capability-grants") {
		return "", "", false
	}
	resourceType := ""
	switch {
	case strings.Contains(path, "/monitoring/alerts"):
		resourceType = "alerting"
	case strings.Contains(path, "/monitoring/logs"):
		resourceType = "logging"
	case strings.Contains(path, "/monitoring"):
		resourceType = "monitoring"
	case strings.Contains(path, "/persistent-volume"):
		resourceType = "storage"
	case strings.Contains(path, "/k8s/ingresses"):
		resourceType = "ingress"
	case strings.Contains(path, "/routes"):
		resourceType = "ingress_route"
	case strings.Contains(path, "/k8s/secrets"):
		resourceType = "secret"
	case strings.Contains(path, "/k8s/configmaps"):
		resourceType = "configmap"
	case strings.Contains(path, "/k8s/services"):
		resourceType = "service"
	case strings.Contains(path, "/k8s/deployments") || strings.Contains(path, "/k8s/statefulsets") || strings.Contains(path, "/k8s/daemonsets"):
		resourceType = "workload"
	case strings.Contains(path, "/k8s/namespaces"):
		resourceType = "namespace"
	case strings.Contains(path, "/certs"):
		resourceType = "certificate"
	case strings.Contains(path, "/chart-repositories"):
		resourceType = "chart_repository"
	case strings.Contains(path, "/platform"):
		resourceType = "platform"
	case strings.Contains(path, "/registry-proxy") || strings.Contains(path, "/registry-proxies"):
		resourceType = "registry_proxy"
	case strings.Contains(path, "/managed-oci-registries"):
		resourceType = "managed_registry"
	case strings.Contains(path, "/image-registries"):
		resourceType = "image_registry"
	case strings.Contains(path, "/system-components"):
		resourceType = "system_component"
	case strings.Contains(path, "/cluster-dns"):
		resourceType = "cluster_dns"
	case strings.Contains(path, "/tailscale"):
		resourceType = "tailnet"
	case strings.Contains(path, "/admin/tables"):
		resourceType = "admin_record"
	case strings.Contains(path, "/environments"):
		resourceType = "environment"
	case strings.Contains(path, "/projects"):
		resourceType = "project"
	case strings.Contains(path, "/applications"):
		resourceType = "application"
	case strings.Contains(path, "/servers"):
		resourceType = "server"
	case strings.Contains(path, "/sites"):
		resourceType = "site"
	case strings.Contains(path, "/domains"):
		resourceType = "domain"
	case strings.Contains(path, "/nodes"):
		resourceType = "cluster_node"
	case strings.Contains(path, "/runtimes"):
		resourceType = "runtime"
	default:
		return "", "", false
	}

	if method == http.MethodGet && resourceType == "secret" && strings.Contains(path, "/:namespace/:name") {
		return "secret.read", resourceType, true
	}
	operation := map[string]string{http.MethodPost: "create", http.MethodPut: "update", http.MethodPatch: "update", http.MethodDelete: "delete"}[method]
	if operation == "" {
		return "", "", false
	}
	switch {
	case strings.Contains(path, "/rollback"):
		operation = "rollback"
	case strings.Contains(path, "/restarts") || strings.Contains(path, "/restart"):
		operation = "restart"
	case strings.Contains(path, "/scale"):
		operation = "scale"
	case strings.Contains(path, "/uninstall"):
		operation = "uninstall"
	case strings.Contains(path, "/install"):
		operation = "install"
	case strings.Contains(path, "/verify"):
		operation = "verify"
	case strings.Contains(path, "/apply"):
		operation = "apply"
	case strings.Contains(path, "/revert"):
		operation = "revert"
	case strings.Contains(path, "/repair"):
		operation = "repair"
	case strings.Contains(path, "/cleanup"):
		operation = "cleanup"
	case strings.Contains(path, "/migrate") || strings.Contains(path, "/migration"):
		operation = "migrate"
	case strings.Contains(path, "/restore"):
		operation = "restore"
	case strings.Contains(path, "/backup"):
		operation = "backup"
	case strings.Contains(path, "/releases"):
		operation = "release"
	case strings.Contains(path, "/deploy"):
		operation = "deploy"
	case strings.Contains(path, "/delegations") || strings.Contains(path, "/integration-handoffs"):
		operation = "delegate"
	case strings.Contains(path, "/sync-namespace"):
		operation = "sync"
	case strings.Contains(path, "/drain") || strings.Contains(path, "/rejoin") || strings.Contains(path, "/unbind"):
		operation = "operate"
	}
	return resourceType + "." + operation, resourceType, true
}

func extractRouteResourceID(c *gin.Context) uint {
	for _, key := range []string{"id", "projectID", "environmentID", "releaseID", "templateID"} {
		if id, err := ParsePositiveID(c.Param(key)); err == nil && id != 0 {
			return id
		}
	}
	return 0
}

func extractRouteTargetName(c *gin.Context, resourceType string) string {
	if namespace, name := strings.TrimSpace(c.Param("namespace")), strings.TrimSpace(c.Param("name")); namespace != "" && name != "" {
		return namespace + "/" + name
	}
	for _, key := range []string{"name", "chart", "table", "provider", "id", "projectID", "environmentID", "releaseID", "templateID"} {
		if value := strings.TrimSpace(c.Param(key)); value != "" {
			return value
		}
	}
	return model.AuditTargetFallback(resourceType, extractRouteResourceID(c))
}

func RequestID(c *gin.Context) string {
	value, _ := c.Get(auditRequestIDKey)
	requestID, _ := value.(string)
	return requestID
}

func newAuditRequestID() string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		return "request-unavailable"
	}
	return "req_" + hex.EncodeToString(bytes)
}

func buildDetail(method, path string, reqBody, respBody []byte) string {
	detail := map[string]interface{}{"method": method, "path": path}
	if len(reqBody) > 0 {
		var body map[string]interface{}
		if json.Unmarshal(reqBody, &body) == nil {
			detail["request"] = redactAuditValue(body)
		}
	}
	data, _ := json.Marshal(detail)
	return string(data)
}

func redactAuditValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		redacted := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			if key == "content" || isSensitiveAuditKey(key) {
				redacted[key] = "[REDACTED]"
				continue
			}
			redacted[key] = redactAuditValue(item)
		}
		return redacted
	case []interface{}:
		redacted := make([]interface{}, len(typed))
		for index, item := range typed {
			redacted[index] = redactAuditValue(item)
		}
		return redacted
	case string:
		return security.Redact(typed)
	default:
		return value
	}
}

func isSensitiveAuditKey(key string) bool {
	key = strings.ToLower(key)
	return strings.Contains(key, "secret") || strings.Contains(key, "password") || strings.Contains(key, "token") || strings.Contains(key, "private_key")
}
