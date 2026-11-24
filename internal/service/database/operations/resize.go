package operations

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/service/database"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *Service) QueueResize(ctx context.Context, id uint, cpu, memory string) (*model.DatabaseOperation, error) {
	cpu, memory, err := database.ValidateMySQLResources(cpu, memory)
	if err != nil {
		return nil, err
	}
	if cpu == "" {
		return nil, fmt.Errorf("请设置 CPU 和内存上限")
	}
	var item model.DatabaseInstance
	if err = s.store.DB().First(&item, id).Error; err != nil {
		return nil, err
	}
	if item.Mode != "managed" {
		return nil, fmt.Errorf("仅平台独立 MySQL 支持调整资源；RDS 请在云控制台变更规格")
	}
	if s.client == nil {
		return nil, fmt.Errorf("集群客户端未配置")
	}
	client, err := s.client(ctx, item.ClusterID)
	if err != nil {
		return nil, err
	}
	set, err := client.Clientset.AppsV1().StatefulSets(database.Namespace).Get(ctx, database.ResourceName(id), metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("读取实例工作负载失败：%w", err)
	}
	if set.Labels[k8s.ManagedByLabel] != k8s.ManagedByValue {
		return nil, fmt.Errorf("实例工作负载不受平台管理")
	}
	if set.Spec.Replicas == nil || *set.Spec.Replicas != 1 {
		return nil, fmt.Errorf("请先恢复实例部署")
	}
	if item.NodeName != "" {
		node, err := client.Clientset.CoreV1().Nodes().Get(ctx, item.NodeName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("读取实例节点失败：%w", err)
		}
		requestedCPU, requestedMemory := resource.MustParse(cpu), resource.MustParse(memory)
		if node.Status.Allocatable.Cpu().Sign() > 0 && requestedCPU.Cmp(*node.Status.Allocatable.Cpu()) > 0 {
			return nil, fmt.Errorf("CPU 请求超过节点可分配容量")
		}
		if node.Status.Allocatable.Memory().Sign() > 0 && requestedMemory.Cmp(*node.Status.Allocatable.Memory()) > 0 {
			return nil, fmt.Errorf("内存请求超过节点可分配容量")
		}
	}
	req := Request{Kind: "resize", Target: Reference(id), ClusterID: item.ClusterID, NodeName: item.NodeName, CPULimit: cpu, MemoryLimit: memory}
	raw, _ := json.Marshal(req)
	task := &model.DatabaseOperation{Kind: "resize", Target: req.Target, ClusterID: req.ClusterID, NodeName: req.NodeName, Spec: string(raw), Status: "queued", PhaseName: Phases("resize")[0]}
	err = s.store.DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&item, id).Error; err != nil {
			return err
		}
		if err := AssertUnlocked(tx, req.Target); err != nil {
			return err
		}
		if err := tx.Create(task).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.DatabaseOperationLock{Resource: req.Target, TaskID: task.ID}).Error; err != nil {
			return fmt.Errorf("实例已有任务占用")
		}
		return tx.Create(&model.DatabaseOperationLog{TaskID: task.ID, Message: fmt.Sprintf("调整资源：%s 核 / %s → %s 核 / %s；更新内存参数并重启 MySQL，保留数据与复制配置", item.CPULimit, item.MemoryLimit, cpu, memory)}).Error
	})
	return task, err
}
