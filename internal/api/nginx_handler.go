package api

import (
	"net/http"

	"fmt"
	"time"

	"github.com/cylism/cylism-manager/internal/agent"
	"github.com/cylism/cylism-manager/internal/nginx"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	pb "github.com/cylism/cylism-manager/api/proto/agent"
	"golang.org/x/net/context"
)

type NginxHandler struct {
	store *store.Store
	pool  *agent.Pool
}

func NewNginxHandler(s *store.Store, pool *agent.Pool) *NginxHandler {
	return &NginxHandler{store: s, pool: pool}
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

	ac, ok := h.pool.Get(req.ServerID)
	if !ok {
		c.JSON(http.StatusBadGateway, gin.H{"error": "agent not connected"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := ac.Client.NginxGetConfig(ctx, &pb.NginxGetConfigRequest{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("nginx get config: %v", err)})
		return
	}
	if resp.Error != "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": resp.Error})
		return
	}

	sites := nginx.ParseNginxConfig(resp.Config)
	// Transform to frontend-friendly format
	type importedSite struct {
		Domain     string `json:"domain"`
		Port       int    `json:"port"`
		SSLEnabled bool   `json:"ssl_enabled"`
		RootPath   string `json:"root_path"`
		ProxyPass  string `json:"proxy_pass"`
	}
	result := make([]importedSite, 0, len(sites))
	for _, s := range sites {
		result = append(result, importedSite{
			Domain:     s.Domain,
			Port:       s.Port,
			SSLEnabled:  s.SSLEnabled,
			RootPath:   s.RootPath,
			ProxyPass:  s.ProxyPass,
		})
	}

	c.JSON(http.StatusOK, gin.H{"sites": result, "total": len(result)})
}
