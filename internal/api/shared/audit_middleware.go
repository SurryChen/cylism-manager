package shared

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	security "github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/service/auth"
	"github.com/gin-gonic/gin"
)

// AuditMiddleware records successful mutating API calls.
func AuditMiddleware(logs repository.AuditRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isMutatingMethod(c.Request.Method) {
			c.Next()
			return
		}
		bodyBytes, _ := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		writer := &responseBodyWriter{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = writer
		start := time.Now()
		c.Next()
		if c.Writer.Status() >= 200 && c.Writer.Status() < 300 && logs != nil {
			entry := &model.AuditLog{Action: inferAction(c.Request.Method, c.FullPath()), ResourceType: inferResourceType(c.FullPath()), ResourceID: extractResourceID(c.Param("id")), UserID: UserID(c), Detail: buildDetailForRequest(c, bodyBytes, writer.body.Bytes()), CreatedAt: start}
			_ = logs.CreateAuditLog(entry)
		}
	}
}

type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseBodyWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

func inferAction(method, path string) string {
	pathActions := map[string]string{"deploy": "deploy", "issue": "issue", "renew": "renew", "revoke": "revoke", "reload": "reload", "generate": "generate", "import": "import"}
	for keyword, action := range pathActions {
		if matched, _ := regexp.MatchString(keyword, path); matched {
			return action
		}
	}
	switch method {
	case http.MethodPost:
		return "create"
	case http.MethodPut, http.MethodPatch:
		return "update"
	case http.MethodDelete:
		return "delete"
	}
	return method
}

func inferResourceType(path string) string {
	checks := []struct {
		pattern string
		value   string
	}{{"/servers", "server"}, {"/sites.*/(issue|renew|revoke)", "cert"}, {"/sites", "site"}, {"/nginx", "nginx"}, {"/applications", "application"}, {"/projects", "project"}}
	for _, check := range checks {
		if matched, _ := regexp.MatchString(check.pattern, path); matched {
			return check.value
		}
	}
	return "unknown"
}

func extractResourceID(idStr string) uint {
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return 0
	}
	return uint(id)
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

func buildDetailForRequest(c *gin.Context, reqBody, respBody []byte) string {
	var detail map[string]interface{}
	_ = json.Unmarshal([]byte(buildDetail(c.Request.Method, c.FullPath(), reqBody, respBody)), &detail)
	if strings.Contains(c.FullPath(), "/configmaps/") && c.Request.Method == http.MethodPut {
		detail["request"] = redactAuditValue(detail["request"])
	}
	if delegation, exists := c.Get("delegation"); exists {
		if claims, ok := delegation.(*auth.DelegationClaims); ok {
			detail["delegation_jti"] = claims.JTI
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
