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

// Import 从目标服务器导入 NGINX 配置 — K3s 版本待实现
func (h *NginxHandler) Import(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"message": "nginx import - K3s implementation pending", "sites": []interface{}{}, "total": 0})
}
