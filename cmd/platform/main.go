package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cylism/cylism-manager/internal/agent"
	"github.com/cylism/cylism-manager/internal/api"
	"github.com/cylism/cylism-manager/internal/auth"
	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/service/server"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func main() {
	// 加载配置
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}

	// 初始化数据库
	dbPath := viper.GetString("database.path")
	if dbPath == "" {
		dbPath = "./data/cylism.db"
	}
	os.MkdirAll("data", 0755)

	db, err := store.New(dbPath)
	if err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}
	log.Println("Database initialized")

	// 管理员初始化
	initAdmin(db)

	// 加密密钥
	encKey := []byte(viper.GetString("encryption.key"))
	if len(encKey) != 32 {
		log.Fatalf("Encryption key must be exactly 32 bytes (got %d)", len(encKey))
	}

	// 加载或生成 TLS CA
	caCertPath := viper.GetString("tls.ca_cert")
	caKeyPath := viper.GetString("tls.ca_key")
	if caCertPath == "" { caCertPath = "data/tls/ca-cert.pem" }
	if caKeyPath == "" { caKeyPath = "data/tls/ca-key.pem" }
	os.MkdirAll("data/tls/servers", 0700)
	_, _, errCa := crypto.LoadOrGenerateCA(caCertPath, caKeyPath)
	if errCa != nil {
		log.Fatalf("Failed to init TLS CA: %v", errCa)
	}
	log.Println("TLS CA initialized")

	// JWT 配置
	jwtSecret := []byte(viper.GetString("auth.jwt_secret"))
	accessTTL := time.Duration(viper.GetInt("auth.access_token_ttl")) * time.Second
	refreshTTL := time.Duration(viper.GetInt("auth.refresh_token_ttl")) * time.Second

	authCfg := &api.AuthConfig{
		JWTSecret:       jwtSecret,
		AccessTokenTTL:  accessTTL,
		RefreshTokenTTL: refreshTTL,
	}

	// Agent 连接池
	pool := agent.NewPool(10 * time.Second)

	// 心跳监控
	heartbeat := server.NewHeartbeatMonitor(db, pool, 30*time.Second)
	heartbeat.Start()
	defer heartbeat.Stop()

	// 操作日志清理任务
	opLogRetention := viper.GetInt("operation_log.retention_days")
	go startOperationLogCleaner(db, time.Duration(opLogRetention))

	// 配置 Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// 注册路由（认证中间件在 router 内部应用）
	api.RegisterRoutes(r, db, encKey, pool, authCfg)

	// 静态文件
	r.Static("/assets", "./web/dist/assets")
	r.NoRoute(func(c *gin.Context) {
		c.File("./web/dist/index.html")
	})

	// 启动服务器
	addr := fmt.Sprintf("%s:%d",
		viper.GetString("server.host"),
		viper.GetInt("server.port"),
	)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		log.Println("Shutting down...")
		pool.CloseAll()
		srv.Close()
	}()

	log.Printf("Cylism Manager Platform starting on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

// startOperationLogCleaner 启动操作日志定时清理
func startOperationLogCleaner(s *store.Store, retentionDays time.Duration) {
	if int(retentionDays) <= 0 {
		log.Println("操作日志清理已禁用（retention_days <= 0）")
		return
	}
	log.Printf("操作日志清理已启动，保留 %d 天", int(retentionDays))
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		if err := s.DeleteExpiredOperationLogs(int(retentionDays)); err != nil {
			log.Printf("操作日志清理失败: %v", err)
		} else {
			log.Println("操作日志清理完成")
		}
	}
}

func initAdmin(s *store.Store) {
	count, err := s.CountUsers()
	if err != nil {
		log.Printf("Warning: failed to count users: %v", err)
		return
	}
	if count > 0 {
		return
	}

	username := viper.GetString("auth.admin_user")
	password := viper.GetString("auth.admin_password")

	hash, err := auth.HashPassword(password)
	if err != nil {
		log.Fatalf("Failed to hash admin password: %v", err)
	}

	user := &model.User{
		Username:     username,
		PasswordHash: hash,
	}
	if err := s.CreateUser(user); err != nil {
		log.Fatalf("Failed to create admin user: %v", err)
	}
	log.Printf("Admin user '%s' created (password from config)", username)
}
