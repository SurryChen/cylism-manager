package api

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strconv"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

// AuditMiddleware 审计日志中间件，记录所有变更类 API 调用
func AuditMiddleware(s *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 只记录变更操作
		if !isMutatingMethod(c.Request.Method) {
			c.Next()
			return
		}

		// 读取请求体
		bodyBytes, _ := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// 记录响应
		writer := &responseBodyWriter{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = writer

		start := time.Now()
		c.Next()

		// 只记录成功的变更
		if c.Writer.Status() >= 200 && c.Writer.Status() < 300 {
			entry := &model.AuditLog{
				Action:       inferAction(c.Request.Method, c.FullPath()),
				ResourceType: inferResourceType(c.FullPath()),
				ResourceID:   extractResourceID(c.Param("id")),
		UserID:       getUserID(c),
				Detail:       buildDetail(c.Request.Method, c.FullPath(), bodyBytes, writer.body.Bytes()),
				CreatedAt:    start,
			}
			_ = s.CreateAuditLog(entry)
		}
	}
}

// responseBodyWriter 捕获响应体
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
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	}
	return false
}

func inferAction(method, path string) string {
	pathActions := map[string]string{
		"deploy":  "deploy",
		"issue":   "issue",
		"renew":   "renew",
		"revoke":  "revoke",
		"reload":  "reload",
		"generate": "generate",
		"import":  "import",
	}
	// 检查路径中的特殊操作
	for keyword, action := range pathActions {
		if matched, _ := regexp.MatchString(keyword, path); matched {
			return action
		}
	}
	switch method {
	case "POST":
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	}
	return method
}

func inferResourceType(path string) string {
	if matched, _ := regexp.MatchString("/servers", path); matched {
		return "server"
	}
	if matched, _ := regexp.MatchString("/sites.*/(issue|renew|revoke)", path); matched {
		return "cert"
	}
	if matched, _ := regexp.MatchString("/sites", path); matched {
		return "site"
	}
	if matched, _ := regexp.MatchString("/nginx", path); matched {
		return "nginx"
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
	detail := map[string]interface{}{
		"method": method,
		"path":   path,
	}
	if len(reqBody) > 0 {
		var body map[string]interface{}
		if json.Unmarshal(reqBody, &body) == nil {
			detail["request"] = body
		}
	}
	data, _ := json.Marshal(detail)
	return string(data)
}

func getUserID(c *gin.Context) uint {
	if id, exists := c.Get("user_id"); exists {
		if uid, ok := id.(uint); ok {
			return uid
		}
	}
	return 0
}
