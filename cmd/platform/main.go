package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cylism/cylism-manager/internal/auth"
	"github.com/cylism/cylism-manager/internal/bootstrap"
	"github.com/cylism/cylism-manager/internal/model"
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

	// 初始化数据库（优先环境变量，兼容 k8s）
	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = viper.GetString("database.path")
	}
	if dbPath == "" {
		dbPath = "/data/cylism.db"
	}
	os.MkdirAll("data", 0755)

	encKey := []byte(viper.GetString("encryption.key"))
	if len(encKey) != 32 {
		log.Fatalf("Encryption key must be exactly 32 bytes (got %d)", len(encKey))
	}
	jwtSecret := []byte(viper.GetString("auth.jwt_secret"))
	accessTTL := time.Duration(viper.GetInt("auth.access_token_ttl")) * time.Second
	refreshTTL := time.Duration(viper.GetInt("auth.refresh_token_ttl")) * time.Second
	container, err := bootstrap.NewContainer(bootstrap.Config{
		DBPath: dbPath, EncryptionKey: encKey, JWTSecret: jwtSecret,
		AccessTokenTTL: accessTTL, RefreshTokenTTL: refreshTTL,
		PlatformURL: viper.GetString("server.public_url"),
	})
	if err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}
	db := container.Store
	log.Println("Database initialized")

	// 管理员初始化
	initAdmin(db)

	// 操作日志清理任务
	opLogRetention := viper.GetInt("operation_log.retention_days")
	appCtx, cancelBackground := context.WithCancel(context.Background())
	defer cancelBackground()
	container.StartBackground(appCtx, time.Duration(opLogRetention))

	if container.K8s != nil {
		log.Println("K8s 客户端已就绪")
	}

	// 配置 Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// 注册路由
	container.RegisterRoutes(r)
	registerFrontendRoutes(r, "./web/dist")

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
		cancelBackground()
		log.Println("Shutting down...")
		srv.Close()
	}()

	log.Printf("Cylism Manager Platform starting on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
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
