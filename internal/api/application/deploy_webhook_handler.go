package applicationapi

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	crypto "github.com/cylism/cylism-manager/internal/security"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const applicationWebhookMaxSkew = 5 * time.Minute

func (h *ApplicationHandler) DeployWebhookStatus(c *gin.Context) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	if _, err := h.queries.GetApplication(id); err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	_, err = h.applications.GetApplicationDeployWebhook(id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.DBError(c, err.Error())
		return
	}
	apiShared.Success(c, gin.H{"configured": err == nil})
}

func (h *ApplicationHandler) GenerateDeployWebhookSecret(c *gin.Context) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	if _, err := h.queries.GetApplication(id); err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		apiShared.InternalError(c, "生成回调密钥失败")
		return
	}
	secret := hex.EncodeToString(raw)
	encrypted, err := crypto.Encrypt(h.encKey, secret)
	if err != nil {
		apiShared.InternalError(c, "加密回调密钥失败")
		return
	}
	if err := h.applications.SaveApplicationDeployWebhook(&model.ApplicationDeployWebhook{ApplicationID: id, EncryptedSecret: encrypted}); err != nil {
		apiShared.InternalError(c, "保存回调密钥失败")
		return
	}
	apiShared.Success(c, gin.H{"secret": secret})
}

func (h *ApplicationHandler) DisableDeployWebhook(c *gin.Context) {
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "应用 ID 无效")
		return
	}
	if _, err := h.queries.GetApplication(id); err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	if err := h.applications.DeleteApplicationDeployWebhook(id); err != nil {
		apiShared.InternalError(c, "关闭自动发布失败")
		return
	}
	apiShared.Success(c, gin.H{"configured": false})
}

// DeployWebhook accepts a signed version from CI and uses the application's
// default template. The caller cannot select another template or image path.
func (h *ApplicationHandler) DeployWebhook(c *gin.Context) {
	unauthorized := func() { apiShared.Unauthorized(c, "发布签名无效或已过期") }
	id, err := apiShared.ParseID(c.Param("id"))
	if err != nil {
		unauthorized()
		return
	}
	webhook, err := h.applications.GetApplicationDeployWebhook(id)
	if err != nil {
		unauthorized()
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, (64<<10)+1))
	if err != nil || len(body) > 64<<10 {
		apiShared.BadRequest(c, "发布请求过大或无法读取")
		return
	}
	secret, err := crypto.Decrypt(h.encKey, webhook.EncryptedSecret)
	if err != nil || !validApplicationWebhookSignature(secret, c.GetHeader("X-Cylism-Timestamp"), c.GetHeader("X-Cylism-Nonce"), c.GetHeader("X-Cylism-Signature"), body) {
		unauthorized()
		return
	}
	var request struct {
		Version string `json:"version"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		apiShared.BadRequest(c, "发布请求无效")
		return
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		apiShared.BadRequest(c, "发布请求无效")
		return
	}
	request.Version = strings.TrimSpace(request.Version)
	if request.Version == "" {
		apiShared.ValidationError(c, "请提供镜像版本号")
		return
	}
	if h.kubernetes == nil || !h.kubernetes.KubernetesAvailable() {
		apiShared.K8sUnavailable(c)
		return
	}
	app, err := h.queries.GetApplication(id)
	if err != nil {
		apiShared.NotFound(c, "应用不存在")
		return
	}
	template, err := h.resources.GetDefaultApplicationDeploymentTemplate(id)
	if err != nil || !template.Enabled {
		apiShared.ValidationError(c, "应用没有可用的默认上线模板")
		return
	}
	workflow := h.releaseWorkflow()
	if _, err := workflow.PrepareTemplateSpec(app, template, request.Version); err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := h.applications.ConsumeApplicationDeployWebhookNonce(id, c.GetHeader("X-Cylism-Nonce"), time.Now()); err != nil {
		unauthorized()
		return
	}
	prepared, err := workflow.CreateFromTemplate(c.Request.Context(), app, template, request.Version, 0)
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	if err := workflow.SyncManagedFiles(app, prepared.Spec, 0); err != nil {
		apiShared.DBError(c, "登记受管文件失败")
		return
	}
	workflow.ExecuteAsync(c.Request.Context(), app, prepared)
	c.JSON(http.StatusAccepted, apiShared.APIResponse{Code: apiShared.CodeSuccess, Message: "自动发布已创建", Data: apiShared.ReleaseDTO(prepared.Release)})
}

func validApplicationWebhookSignature(secret, timestampHeader, nonce, signature string, body []byte) bool {
	if secret == "" || len(nonce) < 16 || len(nonce) > 256 || len(signature) != 64 {
		return false
	}
	timestamp, err := strconv.ParseInt(timestampHeader, 10, 64)
	if err != nil || time.Since(time.Unix(timestamp, 0)).Abs() > applicationWebhookMaxSkew {
		return false
	}
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestampHeader + "." + nonce + "."))
	_, _ = mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}
