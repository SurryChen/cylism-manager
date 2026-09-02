// Package bootstrap owns application composition. It is the only package
// outside cmd/platform that is responsible for wiring infrastructure into the
// HTTP API; domain services remain unaware of this package.
package bootstrap

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/cylism/cylism-manager/internal/api"
	authapi "github.com/cylism/cylism-manager/internal/api/auth"
	systemapi "github.com/cylism/cylism-manager/internal/api/system"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

// Config contains process-level dependencies and authentication settings.
// Database and Kubernetes clients are created by NewContainer.
type Config struct {
	DBPath          string
	EncryptionKey   []byte
	JWTSecret       []byte
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	PlatformURL     string
}

// Container is the application dependency container.
type Container struct {
	Store  *store.Store
	K8s    *k8s.Client
	Auth   *authapi.AuthConfig
	encKey []byte
}

// NewContainer initializes process-owned infrastructure. Kubernetes is
// intentionally best-effort so the API can start in a degraded mode.
func NewContainer(cfg Config) (*Container, error) {
	db, err := NewRepositories(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	client, err := NewKubernetesClient()
	if err != nil {
		log.Printf("WARNING: K8s 客户端不可用: %v（集群相关功能将降级）", err)
	}
	return &Container{
		Store:  db,
		K8s:    client,
		encKey: append([]byte(nil), cfg.EncryptionKey...),
		Auth: &authapi.AuthConfig{
			JWTSecret:       append([]byte(nil), cfg.JWTSecret...),
			AccessTokenTTL:  cfg.AccessTokenTTL,
			RefreshTokenTTL: cfg.RefreshTokenTTL,
			PlatformURL:     cfg.PlatformURL,
		},
	}, nil
}

// RegisterRoutes exposes the composed application through the existing API
// route binder. The global client assignment is kept at this boundary for
// backward compatibility with the remaining infrastructure adapters.
func (c *Container) RegisterRoutes(r *gin.Engine) {
	api.RegisterRoutes(r, c.Store, c.configEncryptionKey(), c.Auth, c.K8s)
}

// Handler returns the HTTP handler after routes have been registered.
func (c *Container) Handler(r *gin.Engine) http.Handler { return r }

func (c *Container) configEncryptionKey() []byte { return append([]byte(nil), c.encKey...) }

// StartBackground starts application-owned reconciliation and maintenance
// loops. The caller only supplies lifecycle configuration; concrete handlers
// and adapters are assembled inside the composition root.
func (c *Container) StartBackground(ctx context.Context, operationLogRetention time.Duration) {
	if ctx == nil {
		ctx = context.Background()
	}
	go c.startOperationLogCleaner(ctx, operationLogRetention)

	var adapter systemapi.SystemComponentAdapter
	if c.K8s != nil {
		adapter = k8s.SystemComponentKubernetesAdapter{Client: c.K8s}
	}
	go func() {
		_ = systemapi.NewSystemComponentHandler(c.Store, adapter).Run(ctx, 5*time.Minute)
	}()
}

func (c *Container) startOperationLogCleaner(ctx context.Context, retention time.Duration) {
	if int(retention) <= 0 {
		log.Println("操作日志清理已禁用（retention_days <= 0）")
		return
	}
	log.Printf("操作日志清理已启动，保留 %d 天", int(retention))
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.Store.DeleteExpiredOperationLogs(int(retention)); err != nil {
				log.Printf("操作日志清理失败: %v", err)
			} else {
				log.Println("操作日志清理完成")
			}
		}
	}
}
