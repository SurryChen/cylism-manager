// Package operations implements independent, durable database capabilities.
package operations

import (
	"fmt"
	"strconv"
	"strings"
)

const DefaultImage = "container-registry.oracle.com/mysql/community-operator:8.4.9-2.1.11"

// References keep external instances separate from Operator-owned clusters.
func ParseReference(value string) (string, uint, error) {
	kind, raw, ok := strings.Cut(value, ":")
	id, err := strconv.ParseUint(raw, 10, 32)
	if !ok || err != nil || id == 0 || (kind != "instance" && kind != "cluster") || strconv.FormatUint(id, 10) != raw {
		return "", 0, fmt.Errorf("数据库资源标识无效")
	}
	return kind, uint(id), nil
}

func Reference(id uint) string { return fmt.Sprintf("instance:%d", id) }

type SwitchConnection struct {
	BindingID uint   `json:"binding_id"`
	Database  string `json:"database"`
	Username  string `json:"username"`
	Password  string `json:"password,omitempty"`
	Revision  uint   `json:"revision"`
}

type Request struct {
	Kind                string             `json:"kind"`
	Source              string             `json:"source"`
	Target              string             `json:"target"`
	ClusterID           uint               `json:"cluster_id"`
	NodeName            string             `json:"node_name"`
	Schemas             []string           `json:"schemas,omitempty"`
	WritesStopped       bool               `json:"writes_stopped"`
	StorageSize         string             `json:"storage_size,omitempty"`
	StorageClass        string             `json:"storage_class,omitempty"`
	Image               string             `json:"image,omitempty"`
	ReplicationUser     string             `json:"replication_user,omitempty"`
	ReplicationPassword string             `json:"replication_password,omitempty"`
	Connections         []SwitchConnection `json:"connections,omitempty"`
}

type endpoint struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
}

type jobInput struct {
	Kind                string             `json:"kind"`
	Source              *endpoint          `json:"source,omitempty"`
	Target              *endpoint          `json:"target,omitempty"`
	Schemas             []string           `json:"schemas,omitempty"`
	ReplicationUser     string             `json:"replication_user,omitempty"`
	ReplicationPassword string             `json:"replication_password,omitempty"`
	Channel             string             `json:"channel"`
	Connections         []SwitchConnection `json:"connections,omitempty"`
}

func Phases(kind string) []string {
	switch kind {
	case "deploy":
		return []string{"提交独立 MySQL 工作负载", "等待 MySQL 就绪"}
	case "inspect":
		return []string{"验证管理连接并读取数据库清单"}
	case "migrate":
		return []string{"检查源库与目标空库", "导出一致性快照", "导入所选数据库", "核对数据库对象"}
	case "replicate":
		return []string{"检查 GTID、空库与复制条件", "导出完整业务快照", "初始化目标与 GTID", "建立只读复制", "验证复制线程"}
	case "detach":
		return []string{"解除指定复制通道（保留只读与数据）"}
	case "replication_check":
		return []string{"检查复制线程与延迟"}
	case "switch":
		return []string{"验证目标业务连接", "更新所选上线模板（待发布生效）"}
	}
	return nil
}
