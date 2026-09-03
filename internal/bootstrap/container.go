// Package bootstrap owns application composition. It is the only package
// outside cmd/platform that is responsible for wiring infrastructure into the
// HTTP API; domain services remain unaware of this package.
package bootstrap

import (
	"context"
	"log"
	"net/http"
	"sync"
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
	Store        *store.Store
	Repositories Repositories
	K8s          *k8s.Client
	Adapters     KubernetesAdapters
	Services     Services
	Auth         *authapi.AuthConfig
	components   *systemapi.SystemComponentHandler
	encKey       []byte
	backgroundMu sync.Mutex
	background   *BackgroundTasks
}

// BackgroundConfig controls Container-owned periodic work. A zero interval
// uses the documented default; a non-positive log retention disables cleanup.
type BackgroundConfig struct {
	OperationLogRetentionDays        int
	OperationLogCleanupInterval      time.Duration
	PlatformReconcileInterval        time.Duration
	RegistryProxyReconcileInterval   time.Duration
	SystemComponentReconcileInterval time.Duration
}

func (c BackgroundConfig) normalized() BackgroundConfig {
	if c.OperationLogCleanupInterval <= 0 {
		c.OperationLogCleanupInterval = time.Hour
	}
	if c.PlatformReconcileInterval <= 0 {
		c.PlatformReconcileInterval = time.Minute
	}
	if c.RegistryProxyReconcileInterval <= 0 {
		c.RegistryProxyReconcileInterval = time.Minute
	}
	if c.SystemComponentReconcileInterval <= 0 {
		c.SystemComponentReconcileInterval = 5 * time.Minute
	}
	return c
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
	repos := BuildRepositories(db)
	container := &Container{
		Store:        db,
		Repositories: repos,
		K8s:          client,
		Adapters:     BuildKubernetesAdapters(client),
		encKey:       append([]byte(nil), cfg.EncryptionKey...),
		Auth: &authapi.AuthConfig{
			JWTSecret:       append([]byte(nil), cfg.JWTSecret...),
			AccessTokenTTL:  cfg.AccessTokenTTL,
			RefreshTokenTTL: cfg.RefreshTokenTTL,
			PlatformURL:     cfg.PlatformURL,
		},
	}
	container.Services = BuildServices(repos, client, container.Adapters, cfg.EncryptionKey)
	container.components = systemapi.NewSystemComponentHandlerWithComposedDependencies(container.Store, container.Adapters.SystemComponent, container.Services.SystemComponent, container.Services.SystemComponentList)
	return container, nil
}

// RegisterRoutes exposes the composed application through the existing API
// route binder and passes migrated Kubernetes adapters explicitly.
func (c *Container) RegisterRoutes(r *gin.Engine) {
	api.RegisterRoutes(r, c.BuildRouteDependencies())
}

// Handler returns the HTTP handler after routes have been registered.
func (c *Container) Handler(r *gin.Engine) http.Handler { return r }

func (c *Container) configEncryptionKey() []byte { return append([]byte(nil), c.encKey...) }

// componentHandler returns the singleton system-component handler shared by
// HTTP routes and the background reconciler. Keeping one instance here avoids
// rebuilding a handler with a separate dependency graph at runtime.
func (c *Container) componentHandler() *systemapi.SystemComponentHandler {
	if c.components == nil {
		c.components = systemapi.NewSystemComponentHandlerWithComposedDependencies(c.Store, c.Adapters.SystemComponent, c.Services.SystemComponent, c.Services.SystemComponentList)
	}
	return c.components
}

// StartBackground starts every Container-owned maintenance task. Starting a
// second set stops and waits for the old set so periodic jobs cannot overlap.
func (c *Container) StartBackground(parent context.Context, config BackgroundConfig) *BackgroundTasks {
	if c == nil {
		return nil
	}
	config = config.normalized()
	c.backgroundMu.Lock()
	defer c.backgroundMu.Unlock()
	if c.background != nil {
		c.background.Stop()
		c.background.Wait()
	}
	tasks := newBackgroundTasks(parent)
	c.background = tasks

	if c.Services.PlatformRelease != nil {
		tasks.Go(func(ctx context.Context) {
			runPeriodic(ctx, config.PlatformReconcileInterval, func(ctx context.Context) {
				c.Services.PlatformRelease.ReconcileLatest(ctx)
			})
		})
	}
	if c.Services.RegistryProxyReconciler != nil {
		tasks.Go(func(ctx context.Context) {
			runPeriodic(ctx, config.RegistryProxyReconcileInterval, func(ctx context.Context) {
				if err := c.Services.RegistryProxyReconciler.Reconcile(ctx); err != nil {
					log.Printf("镜像代理后台协调失败: %v", err)
				}
			})
		})
	}
	if service := c.Services.SystemComponent; service != nil {
		tasks.Go(func(ctx context.Context) {
			if err := service.Run(ctx, config.SystemComponentReconcileInterval); err != nil && ctx.Err() == nil {
				log.Printf("系统组件后台协调停止: %v", err)
			}
		})
	}
	if config.OperationLogRetentionDays > 0 && c.Store != nil {
		tasks.Go(func(ctx context.Context) {
			runPeriodic(ctx, config.OperationLogCleanupInterval, func(context.Context) {
				c.cleanOperationLogs(config.OperationLogRetentionDays)
			})
		})
	}
	return tasks
}

func (c *Container) cleanOperationLogs(retentionDays int) {
	if retentionDays <= 0 || c == nil || c.Store == nil {
		log.Println("操作日志清理已禁用（retention_days <= 0）")
		return
	}
	if err := c.Store.DeleteExpiredOperationLogs(retentionDays); err != nil {
		log.Printf("操作日志清理失败: %v", err)
	} else {
		log.Println("操作日志清理完成")
	}
}
