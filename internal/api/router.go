package api

import (
	"fmt"
	"log"
	"time"

	"github.com/cylism/cylism-manager/internal/agent"
	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/service/deployer"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

// AuthConfig 认证相关配置
type AuthConfig struct {
	JWTSecret       []byte
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	AdminUser       string
	AdminPassword   string
}

// RegisterRoutes 注册所有 API 路由
func RegisterRoutes(r *gin.Engine, s *store.Store, encKey []byte, pool *agent.Pool, authCfg *AuthConfig) {
	// 认证路由
	authHandler := NewAuthHandler(s, authCfg.JWTSecret, authCfg.AccessTokenTTL, authCfg.RefreshTokenTTL)
	authGroup := r.Group("/api/auth")
	{
		authGroup.POST("/login", authHandler.Login)
		authGroup.POST("/refresh", authHandler.Refresh)
		authGroup.GET("/me", JWTAuthMiddleware(authCfg.JWTSecret), authHandler.Me)
	}

	// 业务 API（受 JWT 保护）
	apiGroup := r.Group("/api")
	apiGroup.Use(JWTAuthMiddleware(authCfg.JWTSecret))
	apiGroup.Use(AuditMiddleware(s))

	dashHandler := NewDashboardHandler(s)
	apiGroup.GET("/dashboard", dashHandler.Get)

	deploySvc := &deployServiceImpl{store: s, pool: pool, encKey: encKey}
	serverHandler := NewServerHandler(s, encKey, deploySvc)
	servers := apiGroup.Group("/servers")
	{
		servers.POST("", serverHandler.Create)
		servers.GET("", serverHandler.List)
		servers.GET("/:id", serverHandler.Get)
		servers.PUT("/:id", serverHandler.Update)
		servers.DELETE("/:id", serverHandler.Delete)
		servers.POST("/:id/deploy", serverHandler.Deploy)
		servers.POST("/:id/ssh-test", serverHandler.TestSSH)
	}

	siteHandler := NewSiteHandler(s)
	sites := apiGroup.Group("/sites")
	{
		sites.POST("", siteHandler.Create)
		sites.GET("", siteHandler.List)
		sites.GET("/:id", siteHandler.Get)
		sites.PUT("/:id", siteHandler.Update)
		sites.DELETE("/:id", siteHandler.Delete)
		sites.POST("/:id/issue", siteHandler.IssueCert)
		sites.POST("/:id/renew", siteHandler.RenewCert)
		sites.POST("/:id/revoke", siteHandler.RevokeCert)
		sites.POST("/:id/nginx/generate", siteHandler.GenerateNginx)
		sites.POST("/:id/nginx/reload", siteHandler.ReloadNginx)
	}

	nginxHandler := NewNginxHandler(s)
	apiGroup.POST("/nginx/import", nginxHandler.Import)

	auditHandler := NewAuditHandler(s)
	apiGroup.GET("/audit-logs", auditHandler.List)
}

// deployServiceImpl 部署服务实现
type deployServiceImpl struct {
	store  *store.Store
	pool   *agent.Pool
	encKey []byte
}

func (d *deployServiceImpl) DeployAgent(server *model.Server) error {
	// 解密 SSH 凭据
	password, _ := crypto.Decrypt(d.encKey, server.SSHPassword)
	key, _ := crypto.Decrypt(d.encKey, server.SSHKey)
	passphrase, _ := crypto.Decrypt(d.encKey, server.SSHKeyPassphrase)

	client, err := deployer.NewSSHClient(server, password, key, passphrase)
	if err != nil {
		return fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()

	// 探测 OS/Arch
	osName, arch, err := client.DetectOSArch()
	if err != nil {
		return fmt.Errorf("探测系统信息失败: %w", err)
	}
	log.Printf("Server %s: OS=%s Arch=%s", server.Name, osName, arch)

	// 选择对应的 Agent 二进制
	agentBin := fmt.Sprintf("bin/agent-%s-%s", osName, arch)
	if osName == "linux" && arch == "amd64" {
		agentBin = "bin/agent-linux-amd64"
	} else if osName == "linux" && arch == "arm64" {
		agentBin = "bin/agent-linux-arm64"
	} else {
		agentBin = "bin/agent"
	}

	// 部署 Agent
	if err := client.DeployAgent(agentBin); err != nil {
		return fmt.Errorf("部署 Agent 失败: %w", err)
	}

	// 建立 gRPC 连接
	if _, err := d.pool.Connect(server); err != nil {
		return fmt.Errorf("gRPC 连接失败: %w", err)
	}

	return nil
}

func (d *deployServiceImpl) TestSSH(server *model.Server) (string, error) {
	password, _ := crypto.Decrypt(d.encKey, server.SSHPassword)
	key, _ := crypto.Decrypt(d.encKey, server.SSHKey)
	passphrase, _ := crypto.Decrypt(d.encKey, server.SSHKeyPassphrase)

	client, err := deployer.NewSSHClient(server, password, key, passphrase)
	if err != nil {
		return "", fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()

	return "SSH 连接成功", nil
}
