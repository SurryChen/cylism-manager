package api

import (
	"github.com/cylism/cylism-manager/internal/model"

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
	model.SuccessWithMessage(c, nil, "nginx import - K3s implementation pending")
}
