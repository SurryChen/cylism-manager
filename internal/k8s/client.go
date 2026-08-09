package k8s

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

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
		ctx:           context.Background(),
	}, nil
}

// Ctx 返回客户端上下文
func (c *Client) Ctx() context.Context {
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func (c *Client) dynamicClient() (dynamic.Interface, error) {
	if c.DynamicClient != nil {
		return c.DynamicClient, nil
	}
	if c.Config == nil {
		return nil, fmt.Errorf("Kubernetes dynamic client 未初始化")
	}
	return dynamic.NewForConfig(c.Config)
}

// CheckCRD 检测指定 CRD 是否存在
func (c *Client) CheckCRD(name string) (bool, error) {
	dynamicClient, err := c.dynamicClient()
	if err != nil {
		return false, err
	}
	crdGVR := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}
	_, err = dynamicClient.Resource(crdGVR).Get(c.Ctx(), name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// CheckRequiredCRDs 检测所有必需的 CRD
func (c *Client) CheckRequiredCRDs() (traefikOK, certManagerOK bool, err error) {
	traefikOK, err = c.CheckCRD("ingressroutes.traefik.io")
	if err != nil {
		return false, false, err
	}
	certManagerOK, err = c.CheckCRD("certificates.cert-manager.io")
	if err != nil {
		return traefikOK, false, err
	}
	return
}
