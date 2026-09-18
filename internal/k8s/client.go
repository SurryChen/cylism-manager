package k8s

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Client K8s 客户端封装
type Client struct {
	Clientset     kubernetes.Interface
	DynamicClient dynamic.Interface
	Config        *rest.Config
	ctx           context.Context

	ingressControllerMu          sync.Mutex
	ingressControllerCache       *IngressControllerStatus
	ingressControllerCacheExpiry time.Time
}

func (c *Client) GetNamespace(ctx context.Context, name string) (*corev1.Namespace, error) {
	return c.Clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
}
func (c *Client) UpdateNamespace(ctx context.Context, namespace *corev1.Namespace) (*corev1.Namespace, error) {
	return c.Clientset.CoreV1().Namespaces().Update(ctx, namespace, metav1.UpdateOptions{})
}
func (c *Client) CreateNamespace(ctx context.Context, namespace *corev1.Namespace) (*corev1.Namespace, error) {
	return c.Clientset.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
}

// KubernetesAvailable reports whether the typed clientset is ready for
// remote operations. It is exposed as a capability method so API handlers do
// not depend on the concrete Kubernetes client type.
func (c *Client) KubernetesAvailable() bool { return c != nil && c.Clientset != nil }

// ServerVersionContext returns the API server version used for optional
// distribution-specific capabilities.
func (c *Client) ServerVersionContext(ctx context.Context) (string, error) {
	if c == nil || c.Clientset == nil {
		return "", fmt.Errorf("Kubernetes client 未初始化")
	}
	version, err := c.Clientset.Discovery().ServerVersion()
	if err != nil {
		return "", err
	}
	return version.GitVersion, nil
}

// NewClient 创建 K8s 客户端，支持 InCluster（生产）和 kubeconfig（开发）双模式
func NewClient() (*Client, error) {
	// 1. 优先尝试 InClusterConfig（Pod 内自动注入 ServiceAccount）
	cfg, err := rest.InClusterConfig()
	if err == nil {
		log.Println("K8s: 使用 InCluster 配置")
		return newClientFromRestConfig(cfg)
	}
	log.Printf("K8s: InCluster 不可用 (%v)，尝试 kubeconfig / 环境变量", err)

	// 2. fallback: 环境变量 KUBERNETES_SERVICE_HOST（开发环境手动设置）
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	port := os.Getenv("KUBERNETES_SERVICE_PORT")
	if host != "" && port != "" {
		log.Printf("K8s: 使用环境变量 %s:%s", host, port)
		return newClientFromRestConfig(&rest.Config{
			Host: fmt.Sprintf("https://%s:%s", host, port),
			TLSClientConfig: rest.TLSClientConfig{
				Insecure: true,
			},
		})
	}

	if host == "" {
		host = os.Getenv("K8S_API_HOST")
	}
	if port == "" {
		port = os.Getenv("K8S_API_PORT")
		if port == "" {
			port = "6443"
		}
	}
	if host != "" {
		log.Printf("K8s: 使用自定义地址 %s:%s", host, port)
		return newClientFromRestConfig(&rest.Config{
			Host: fmt.Sprintf("https://%s:%s", host, port),
			TLSClientConfig: rest.TLSClientConfig{
				Insecure: true,
			},
		})
	}

	// 3. 最后尝试本地默认 k3s 地址
	log.Println("K8s: 尝试本地 K3s 默认地址 (127.0.0.1:6443)")
	return newClientFromRestConfig(&rest.Config{
		Host: "https://127.0.0.1:6443",
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	})
}

// newClientFromRestConfig 从 rest.Config 创建客户端
func newClientFromRestConfig(config *rest.Config) (*Client, error) {
	if config == nil {
		config = &rest.Config{Host: "localhost:8080"}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	return &Client{
		Clientset:     clientset,
		DynamicClient: dynamicClient,
		Config:        config,
	}, nil
}

// withContext returns an isolated client view carrying the caller's
// cancellation boundary. The underlying clients are immutable handles, so
// cloning avoids mutating the shared process client while allowing legacy
// resource implementations to honor request cancellation.
func (c *Client) withContext(ctx context.Context) *Client {
	if c == nil {
		return nil
	}
	clone := *c
	if ctx == nil {
		return nil
	}
	clone.ctx = ctx
	return &clone
}

func (c *Client) dynamicClient() (dynamic.Interface, error) {
	if c == nil {
		return nil, fmt.Errorf("Kubernetes dynamic client 未初始化")
	}
	if c.DynamicClient != nil {
		return c.DynamicClient, nil
	}
	if c.Config == nil {
		return nil, fmt.Errorf("Kubernetes dynamic client 未初始化")
	}
	return dynamic.NewForConfig(c.Config)
}

// CheckCRDContext checks a CRD using the caller's cancellation boundary.
func (c *Client) CheckCRDContext(ctx context.Context, name string) (bool, error) {
	dynamicClient, err := c.dynamicClient()
	if err != nil {
		return false, err
	}
	crdGVR := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}
	_, err = dynamicClient.Resource(crdGVR).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// CheckRequiredCRDsContext 检测所有必需的 CRD。
func (c *Client) CheckRequiredCRDsContext(ctx context.Context) (traefikOK, certManagerOK bool, err error) {
	traefikOK, err = c.CheckCRDContext(ctx, "ingressroutes.traefik.io")
	if err != nil {
		return false, false, err
	}
	certManagerOK, err = c.CheckCRDContext(ctx, "certificates.cert-manager.io")
	if err != nil {
		return traefikOK, false, err
	}
	return
}
