package api

import (
	"net/http"

	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

type NginxHandler struct {
	store *store.Store
}

func NewNginxHandler(s *store.Store) *NginxHandler {
	return &NginxHandler{store: s}
}

// Import 从目标服务器导入 NGINX 配置 POST /api/nginx/import
func (h *NginxHandler) Import(c *gin.Context) {
	var req struct {
		ServerID uint `json:"server_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// TODO: 通过 Agent 调用 nginx -T，解析 server{} 块，创建 unmanaged site
	c.JSON(http.StatusOK, gin.H{"message": "nginx import - not implemented yet", "server_id": req.ServerID})
}
