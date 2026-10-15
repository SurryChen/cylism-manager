package database

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const MyCNFRestartAnnotation = "cylism.io/mysql-config-revision"

var mysqlConfigOptions = map[string]bool{
	"innodb_buffer_pool_size":         true,
	"innodb_log_buffer_size":          true,
	"temptable_max_ram":               true,
	"tmp_table_size":                  true,
	"max_heap_table_size":             true,
	"sort_buffer_size":                true,
	"join_buffer_size":                true,
	"read_buffer_size":                true,
	"read_rnd_buffer_size":            true,
	"max_connections":                 true,
	"performance_schema_digests_size": true,
	"performance_schema_events_statements_history_long_size": true,
	"performance_schema_max_sql_text_length":                 true,
	"performance_schema_max_digest_length":                   true,
}

var mysqlConfigNumber = regexp.MustCompile(`^[0-9]+([KMG])?$`)

func ValidateMyCNF(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
	if len(value) > 16*1024 {
		return "", fmt.Errorf("MySQL 配置不能超过 16 KiB")
	}
	if value == "" {
		return "", nil
	}
	section := ""
	seen := map[string]bool{}
	for i, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			if section != "mysqld" {
				return "", fmt.Errorf("第 %d 行：仅支持 [mysqld] 配置段", i+1)
			}
			continue
		}
		if section != "mysqld" {
			return "", fmt.Errorf("第 %d 行：请先添加 [mysqld] 配置段", i+1)
		}
		key, option, ok := strings.Cut(line, "=")
		key = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
		if !ok || key == "" || strings.TrimSpace(option) == "" || strings.ContainsAny(key, " \t!/") {
			return "", fmt.Errorf("第 %d 行：配置项须为 name=value 格式", i+1)
		}
		if seen[key] {
			return "", fmt.Errorf("第 %d 行：配置项 %s 重复", i+1, key)
		}
		seen[key] = true
		if !mysqlConfigOptions[key] {
			return "", fmt.Errorf("第 %d 行：暂不支持修改 %s", i+1, key)
		}
		if !mysqlConfigNumber.MatchString(strings.ToUpper(strings.TrimSpace(option))) {
			return "", fmt.Errorf("第 %d 行：%s 须为非负整数或 K/M/G 大小", i+1, key)
		}
	}
	return value + "\n", nil
}

func MyCNFRevision(value string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
}

func MyCNFState(ctx context.Context, client *k8s.Client, item *model.MySQLCluster) (string, bool, error) {
	if item.Status == "draft" || item.Status == "paused" {
		return item.MyCNF, false, nil
	}
	resource, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil {
		return "", false, fmt.Errorf("读取 MySQL 集群配置: %w", err)
	}
	value, _, err := unstructured.NestedString(resource.Object, "spec", "mycnf")
	if err != nil {
		return "", false, err
	}
	return value, item.ConfigRestartRequired, nil
}

func UpdateMyCNF(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, value string) error {
	resource := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace)
	current, err := resource.Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("读取 MySQL 集群: %w", err)
	}
	if current.GetLabels()[k8s.ManagedByLabel] != k8s.ManagedByValue {
		return fmt.Errorf("该 MySQL 集群不受平台管理")
	}
	if value == "" {
		unstructured.RemoveNestedField(current.Object, "spec", "mycnf")
	} else if err := unstructured.SetNestedField(current.Object, value, "spec", "mycnf"); err != nil {
		return err
	}
	_, err = resource.Update(ctx, current, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("更新 Operator MySQL 配置: %w", err)
	}
	return nil
}

func RestartMySQLForConfig(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, value string) error {
	sets := client.Clientset.AppsV1().StatefulSets(item.Namespace)
	sts, err := sets.Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("读取 MySQL 工作负载: %w", err)
	}
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas < 1 {
		return fmt.Errorf("MySQL 工作负载没有运行中的实例")
	}
	if sts.Spec.Template.Annotations == nil {
		sts.Spec.Template.Annotations = map[string]string{}
	}
	sts.Spec.Template.Annotations[MyCNFRestartAnnotation] = MyCNFRevision(value)
	sts.Spec.Template.Annotations["cylism.io/mysql-config-restarted-at"] = time.Now().UTC().Format(time.RFC3339Nano)
	_, err = sets.Update(ctx, sts, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("提交 MySQL 滚动重启: %w", err)
	}
	return nil
}
