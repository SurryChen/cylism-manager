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
		servers.POST("/:id/deploy/probe", serverHandler.ProbeDeploy)
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

	operationHandler := NewOperationHandler(s)
	apiGroup.GET("/operations", operationHandler.ListOperations)

	dbAdminHandler := NewDBAdminHandler(s)
	adminGroup := apiGroup.Group("/admin/tables")
	{
		adminGroup.GET("", dbAdminHandler.ListTables)
		adminGroup.GET("/:table", dbAdminHandler.ListRecords)
		adminGroup.POST("/:table", dbAdminHandler.CreateRecord)
		adminGroup.PUT("/:table/:id", dbAdminHandler.UpdateRecord)
		adminGroup.DELETE("/:table/:id", dbAdminHandler.DeleteRecord)
	}

	auditHandler := NewAuditHandler(s)
	apiGroup.GET("/audit-logs", auditHandler.List)
}

// deployServiceImpl 部署服务实现
type deployServiceImpl struct {
	store  *store.Store
	pool   *agent.Pool
	encKey []byte
}

func (d *deployServiceImpl) ProbeAgent(server *model.Server) (*deployer.AgentProbeResult, error) {
	password, _ := crypto.Decrypt(d.encKey, server.SSHPassword)
	key, _ := crypto.Decrypt(d.encKey, server.SSHKey)
	passphrase, _ := crypto.Decrypt(d.encKey, server.SSHKeyPassphrase)

	client, err := deployer.NewSSHClient(server, password, key, passphrase)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()

	return client.ProbeAgent(), nil
}

func (d *deployServiceImpl) DeployAgent(server *model.Server, force bool) error {
	// 操作日志辅助函数
	addLog := func(step, status, detail string) {
		d.store.CreateOperationLog(&model.OperationLog{
			ResourceType: "server",
			ResourceID:   server.ID,
			Step:         step,
			Status:       status,
			Detail:       detail,
		})
	}

	addLog("正在连接 SSH", "running", "")

	// 解密 SSH 凭据
	password, _ := crypto.Decrypt(d.encKey, server.SSHPassword)
	key, _ := crypto.Decrypt(d.encKey, server.SSHKey)
	passphrase, _ := crypto.Decrypt(d.encKey, server.SSHKeyPassphrase)

	client, err := deployer.NewSSHClient(server, password, key, passphrase)
	if err != nil {
		addLog("正在连接 SSH", "failed", err.Error())
		return fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()
	addLog("正在连接 SSH", "success", "SSH 连接成功")

	// 探测 OS/Arch
	addLog("正在检测操作系统", "running", "")
	osName, arch, err := client.DetectOSArch()
	if err != nil {
		addLog("正在检测操作系统", "failed", err.Error())
		return fmt.Errorf("探测系统信息失败: %w", err)
	}
	addLog("正在检测操作系统", "success", fmt.Sprintf("OS=%s Arch=%s", osName, arch))
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

	// 部署 Agent（上传 + 启动）
	addLog("正在部署 Agent", "running", "")
	// Force 模式：先停止旧的 Agent
	if force {
		addLog("正在停止旧 Agent", "running", "")
		client.RunCmd("systemctl stop cylism-agent 2>/dev/null || true")
		client.RunCmd("pkill -f 'cylism-agent' 2>/dev/null || true")
		addLog("正在停止旧 Agent", "success", "旧 Agent 已停止")
	}

	if err := client.DeployAgent(agentBin); err != nil {
		addLog("正在部署 Agent", "failed", err.Error())
		return fmt.Errorf("部署 Agent 失败: %w", err)
	}
	addLog("正在部署 Agent", "success", "Agent 部署成功")

	// 建立 gRPC 连接
	addLog("正在建立 gRPC 连接", "running", "")
	if _, err := d.pool.Connect(server); err != nil {
		addLog("正在建立 gRPC 连接", "failed", err.Error())
		return fmt.Errorf("gRPC 连接失败: %w", err)
	}
	addLog("正在建立 gRPC 连接", "success", "gRPC 连接成功")

	return nil
}

func (d *deployServiceImpl) TestSSH(server *model.Server) (string, error) {
	addLog := func(step, status, detail string) {
		d.store.CreateOperationLog(&model.OperationLog{
			ResourceType: "server",
			ResourceID:   server.ID,
			Step:         step,
			Status:       status,
			Detail:       detail,
		})
	}

	addLog("正在测试 SSH 连通性", "running", "")

	password, _ := crypto.Decrypt(d.encKey, server.SSHPassword)
	key, _ := crypto.Decrypt(d.encKey, server.SSHKey)
	passphrase, _ := crypto.Decrypt(d.encKey, server.SSHKeyPassphrase)

	client, err := deployer.NewSSHClient(server, password, key, passphrase)
	if err != nil {
		addLog("正在测试 SSH 连通性", "failed", err.Error())
		return "", fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()

	addLog("正在测试 SSH 连通性", "success", "SSH 连接成功")
	return "SSH 连接成功", nil
}
