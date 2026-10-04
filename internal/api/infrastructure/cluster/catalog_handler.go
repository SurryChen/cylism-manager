package cluster

import (
	"errors"
	"strings"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ClusterCatalog interface {
	ListClusters() ([]model.Cluster, error)
	GetCluster(uint) (*model.Cluster, error)
	GetClusterByName(string) (*model.Cluster, error)
	CreateCluster(*model.Cluster) error
	UpdateClusterName(uint, string) (*model.Cluster, error)
	DeleteCluster(uint) error
}

type CatalogHandler struct {
	catalog ClusterCatalog
}

type clusterSummary struct {
	ID              uint      `json:"id"`
	Name            string    `json:"name"`
	Distribution    string    `json:"distribution"`
	Status          string    `json:"status"`
	ControlServerID *uint     `json:"control_server_id,omitempty"`
	ControlAddress  string    `json:"control_address,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

func NewCatalogHandler(catalog ClusterCatalog) *CatalogHandler {
	return &CatalogHandler{catalog: catalog}
}

func summarizeCluster(cluster *model.Cluster) clusterSummary {
	return clusterSummary{
		ID: cluster.ID, Name: cluster.Name, Distribution: cluster.Distribution,
		Status: cluster.Status, ControlServerID: cluster.ControlServerID,
		ControlAddress: cluster.ControlAddress, CreatedAt: cluster.CreatedAt,
	}
}

func (h *CatalogHandler) List(c *gin.Context) {
	clusters, err := h.catalog.ListClusters()
	if err != nil {
		apiShared.InternalError(c, "加载集群列表失败")
		return
	}
	summaries := make([]clusterSummary, 0, len(clusters))
	for index := range clusters {
		summaries = append(summaries, summarizeCluster(&clusters[index]))
	}
	apiShared.Success(c, summaries)
}

func (h *CatalogHandler) Get(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "集群 ID 无效")
		return
	}
	cluster, err := h.catalog.GetCluster(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "集群不存在")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "加载集群失败")
		return
	}
	apiShared.Success(c, summarizeCluster(cluster))
}

func (h *CatalogHandler) Create(c *gin.Context) {
	var request struct {
		Name         string `json:"name"`
		Distribution string `json:"distribution"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "集群名称无效")
		return
	}
	name := strings.TrimSpace(request.Name)
	if name == "" || len([]rune(name)) > 128 {
		apiShared.BadRequest(c, "集群名称应为 1 至 128 个字符")
		return
	}
	if request.Distribution == "" {
		apiShared.BadRequest(c, "请选择集群类型")
		return
	}
	if request.Distribution == "kubernetes" {
		apiShared.BadRequest(c, "目前仅支持创建 K3s 集群，K8s 暂未开放")
		return
	}
	if request.Distribution != "k3s" {
		apiShared.BadRequest(c, "集群类型不支持")
		return
	}
	cluster := &model.Cluster{Name: name, Distribution: request.Distribution, Status: "draft"}
	if err := h.catalog.CreateCluster(cluster); err != nil {
		if _, lookupErr := h.catalog.GetClusterByName(name); lookupErr == nil {
			apiShared.Conflict(c, "集群名称已存在")
			return
		}
		apiShared.InternalError(c, "创建集群失败")
		return
	}
	apiShared.Success(c, summarizeCluster(cluster))
}

func (h *CatalogHandler) Update(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "集群 ID 无效")
		return
	}
	var request struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "集群名称无效")
		return
	}
	name := strings.TrimSpace(request.Name)
	if name == "" || len([]rune(name)) > 128 {
		apiShared.BadRequest(c, "集群名称应为 1 至 128 个字符")
		return
	}
	cluster, err := h.catalog.UpdateClusterName(id, name)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "集群不存在")
		return
	}
	if err != nil {
		if existing, lookupErr := h.catalog.GetClusterByName(name); lookupErr == nil && existing.ID != id {
			apiShared.Conflict(c, "集群名称已存在")
			return
		}
		apiShared.InternalError(c, "修改集群失败")
		return
	}
	apiShared.Success(c, summarizeCluster(cluster))
}

func (h *CatalogHandler) Delete(c *gin.Context) {
	id, err := apiShared.ParsePositiveID(c.Param("id"))
	if err != nil {
		apiShared.BadRequest(c, "集群 ID 无效")
		return
	}
	err = h.catalog.DeleteCluster(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		apiShared.NotFound(c, "集群不存在")
		return
	}
	if errors.Is(err, store.ErrClusterHasNodes) {
		apiShared.Conflict(c, "请先卸载集群中的全部节点，等待任务结束后再删除集群")
		return
	}
	if err != nil {
		apiShared.InternalError(c, "删除集群失败")
		return
	}
	apiShared.Success(c, gin.H{"deleted": true})
}
