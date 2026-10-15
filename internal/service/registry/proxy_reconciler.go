package registry

import (
	"context"
	"fmt"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

// ProxyStatusReader is the narrow Kubernetes capability required to observe a
// Registry Proxy deployment.
type ProxyStatusReader interface {
	Available() bool
	DeploymentAvailable(context.Context, *model.RegistryProxy) (available, missing bool, err error)
}

// ProxyCacheCleaner is the narrow Kubernetes capability required for periodic
// Registry Proxy cache cleanup.
type ProxyCacheCleaner interface {
	ClearCache(context.Context, *model.RegistryProxy) error
}

// ProxyReconciler keeps persisted Registry Proxy status and cache cleanup in a
// service so HTTP requests and background tasks share the same semantics.
type ProxyReconciler struct {
	service *ProxyService
	status  ProxyStatusReader
	cleaner ProxyCacheCleaner
	now     func() time.Time
	resolve func(context.Context, *model.RegistryProxy) (ProxyStatusReader, ProxyCacheCleaner, error)
}

func NewProxyReconciler(service *ProxyService, status ProxyStatusReader, cleaner ProxyCacheCleaner) *ProxyReconciler {
	return &ProxyReconciler{service: service, status: status, cleaner: cleaner, now: time.Now}
}

func (r *ProxyReconciler) WithResolver(resolve func(context.Context, *model.RegistryProxy) (ProxyStatusReader, ProxyCacheCleaner, error)) *ProxyReconciler {
	r.resolve = resolve
	return r
}

// Reconcile refreshes every saved proxy. A transient failure for one proxy is
// returned to the task owner while later scheduled passes continue running.
func (r *ProxyReconciler) Reconcile(ctx context.Context) error {
	if r == nil || r.service == nil {
		return nil
	}
	proxies, err := r.service.List()
	if err != nil {
		return err
	}
	for index := range proxies {
		if err := r.Refresh(ctx, &proxies[index]); err != nil {
			return err
		}
	}
	return nil
}

// Refresh applies the established Registry Proxy status rules to one proxy.
// It is also used by the HTTP handler before returning a proxy to the browser.
func (r *ProxyReconciler) Refresh(ctx context.Context, proxy *model.RegistryProxy) error {
	if r == nil || proxy == nil || r.service == nil {
		return nil
	}
	status, cleaner := r.status, r.cleaner
	if proxy.ClusterID != 0 {
		if r.resolve == nil {
			return fmt.Errorf("集群 %d 的代理客户端未配置", proxy.ClusterID)
		}
		var err error
		status, cleaner, err = r.resolve(ctx, proxy)
		if err != nil {
			proxy.Status, proxy.LastError = "failed", "读取集群代理状态失败: "+err.Error()
			return r.service.Save(proxy)
		}
	}
	if status == nil || !status.Available() {
		return nil
	}
	available, missing, err := status.DeploymentAvailable(ctx, proxy)
	if missing {
		proxy.Status, proxy.LastError = "missing", "代理 Deployment 不存在"
	} else if err != nil {
		proxy.Status, proxy.LastError = "failed", "读取代理状态失败: "+err.Error()
	} else if available {
		proxy.Status, proxy.LastError = "ready", ""
		if proxy.ManagementMode == "observe" {
			// Imported resources remain under their original controller until explicitly taken over.
		} else if proxy.LastCleanupAt == nil {
			now := r.nowTime()
			proxy.LastCleanupAt = &now
		} else if cleaner != nil && r.nowTime().Sub(*proxy.LastCleanupAt) >= time.Duration(proxy.CleanupIntervalHours)*time.Hour {
			if clearErr := cleaner.ClearCache(ctx, proxy); clearErr == nil {
				now := r.nowTime()
				proxy.LastCleanupAt, proxy.Status, proxy.LastError = &now, "deploying", "定期清理后等待新 Pod 就绪"
			}
		}
	} else {
		proxy.Status = "deploying"
	}
	now := r.nowTime()
	proxy.LastCheckedAt = &now
	if err := r.service.Save(proxy); err != nil {
		return fmt.Errorf("保存代理状态: %w", err)
	}
	return nil
}

func (r *ProxyReconciler) nowTime() time.Time {
	if r.now == nil {
		return time.Now()
	}
	return r.now()
}
