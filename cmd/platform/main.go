package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cylism/cylism-manager/internal/bootstrap"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/service/auth"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

type serverShutdowner interface {
	Shutdown(context.Context) error
}

type backgroundStopper interface {
	Stop()
	Wait()
}

func validateSecurityConfig(encryptionKey, jwtSecret, adminPassword string) error {
	if len(encryptionKey) != 32 {
		return fmt.Errorf("encryption key must be exactly 32 bytes (got %d)", len(encryptionKey))
	}
	if strings.TrimSpace(jwtSecret) == "" {
		return fmt.Errorf("auth.jwt_secret must be configured")
	}
	if strings.TrimSpace(adminPassword) == "" {
		return fmt.Errorf("auth.admin_password must be configured")
	}
	return nil
}

// shutdownPlatform stops HTTP admission first, then cancels and joins
// Container-owned work so no background goroutine outlives the process.
func shutdownPlatform(ctx context.Context, server serverShutdowner, background backgroundStopper) error {
	err := server.Shutdown(ctx)
	if background != nil {
		background.Stop()
		background.Wait()
	}
	return err
}

func main() {
	// 加载配置
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("./config")
	viper.AddConfigPath(".")
	// Environment variables override the optional local configuration file so
	// production images do not need to contain secrets or a generated config.
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.BindEnv("encryption.key", "ENCRYPTION_KEY")
	viper.BindEnv("auth.admin_user", "ADMIN_USER")
	viper.BindEnv("auth.admin_password", "ADMIN_PASSWORD")
	viper.BindEnv("auth.jwt_secret", "JWT_SECRET")
	viper.BindEnv("server.host", "SERVER_HOST")
	viper.BindEnv("server.port", "SERVER_PORT")
	viper.BindEnv("server.public_url", "PUBLIC_URL")
	viper.BindEnv("database.path", "DATABASE_PATH")
	viper.BindEnv("auth.access_token_ttl", "ACCESS_TOKEN_TTL")
	viper.BindEnv("auth.refresh_token_ttl", "REFRESH_TOKEN_TTL")
	viper.BindEnv("operation_log.retention_days", "OPERATION_LOG_RETENTION_DAYS")
	viper.SetDefault("server.host", "0.0.0.0")
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("database.path", "/data/cylism.db")
	viper.SetDefault("auth.admin_user", "admin")
	viper.SetDefault("auth.access_token_ttl", 7200)
	viper.SetDefault("auth.refresh_token_ttl", 604800)
	viper.SetDefault("operation_log.retention_days", 30)
	if err := viper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			log.Fatalf("Failed to read config: %v", err)
		}
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

	encryptionKey := viper.GetString("encryption.key")
	jwtSecretValue := viper.GetString("auth.jwt_secret")
	adminPassword := viper.GetString("auth.admin_password")
	if err := validateSecurityConfig(encryptionKey, jwtSecretValue, adminPassword); err != nil {
		log.Fatalf("Invalid security configuration: %v", err)
	}
	encKey := []byte(encryptionKey)
	jwtSecret := []byte(jwtSecretValue)
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

	appCtx, cancelBackground := context.WithCancel(context.Background())
	background := container.StartBackground(appCtx, bootstrap.BackgroundConfig{
		OperationLogRetentionDays: viper.GetInt("operation_log.retention_days"),
	})
	defer func() {
		cancelBackground()
		background.Wait()
	}()

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

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)
	go func() {
		<-quit
		log.Println("Shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := shutdownPlatform(shutdownCtx, srv, background); err != nil {
			log.Printf("HTTP server shutdown failed: %v", err)
		}
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
