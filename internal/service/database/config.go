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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const MyCNFRestartAnnotation = "cylism.io/mysql-config-revision"
const mysqlResourceManagedAnnotation = "cylism.io/mysql-resources-managed"
const mysqlExtraConfigHeader = "# Additional user configurations taken from spec.mycnf in InnoDBCluster.\n# Do not edit directly.\n"

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

var tunedOptions = []string{
	"innodb_buffer_pool_size", "innodb_log_buffer_size",
	"max_connections", "sort_buffer_size", "join_buffer_size", "read_buffer_size",
	"read_rnd_buffer_size", "temptable_max_ram", "tmp_table_size", "max_heap_table_size",
	"performance_schema_digests_size", "performance_schema_events_statements_history_long_size",
}

func ValidateMySQLResources(cpu, memory string) (string, string, error) {
	cpu, memory = strings.TrimSpace(cpu), strings.TrimSpace(memory)
	if cpu == "" && memory == "" {
		return "", "", nil
	}
	if cpu == "" || memory == "" {
		return "", "", fmt.Errorf("请同时设置 MySQL CPU 和内存上限")
	}
	cpuQuantity, cpuErr := resource.ParseQuantity(cpu)
	memoryQuantity, memoryErr := resource.ParseQuantity(memory)
	if cpuErr != nil || memoryErr != nil || cpuQuantity.MilliValue() < 100 || cpuQuantity.MilliValue() > 16000 || memoryQuantity.Value() < 1<<30 || memoryQuantity.Value() > 64<<30 {
		return "", "", fmt.Errorf("MySQL CPU 须为 100m–16 核，内存须为 1Gi–64Gi")
	}
	return cpuQuantity.String(), memoryQuantity.String(), nil
}

// EffectiveMyCNF keeps custom settings while resource-managed options take precedence.
func EffectiveMyCNF(custom, memory string) string {
	if memory == "" {
		return custom
	}
	quantity, err := resource.ParseQuantity(memory)
	if err != nil {
		return custom
	}
	managed := make(map[string]bool, len(tunedOptions))
	for _, option := range tunedOptions {
		managed[option] = true
	}
	lines := []string{"[mysqld]"}
	for _, line := range strings.Split(strings.TrimSpace(custom), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.EqualFold(trimmed, "[mysqld]") {
			continue
		}
		key, _, ok := strings.Cut(trimmed, "=")
		if ok && managed[strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))] {
			continue
		}
		lines = append(lines, line)
	}
	poolMi := quantity.Value() / (4 << 20)
	connections := 50
	if quantity.Value() > 2<<30 {
		connections = 100
	}
	lines = append(lines,
		fmt.Sprintf("innodb_buffer_pool_size=%dM", poolMi),
		"innodb_log_buffer_size=16M",
		fmt.Sprintf("max_connections=%d", connections),
		"sort_buffer_size=512K", "join_buffer_size=512K", "read_buffer_size=512K", "read_rnd_buffer_size=512K",
		"temptable_max_ram=32M", "tmp_table_size=16M", "max_heap_table_size=16M",
		"performance_schema_digests_size=1000", "performance_schema_events_statements_history_long_size=0",
	)
	return strings.Join(lines, "\n") + "\n"
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
	return UpdateMySQLSettings(ctx, client, item, value, item.CPULimit, item.MemoryLimit)
}

func UpdateMySQLSettings(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, value, cpu, memory string) error {
	resource := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace)
	current, err := resource.Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("读取 MySQL 集群: %w", err)
	}
	if current.GetLabels()[k8s.ManagedByLabel] != k8s.ManagedByValue {
		return fmt.Errorf("该 MySQL 集群不受平台管理")
	}
	value = EffectiveMyCNF(value, memory)
	if value == "" {
		unstructured.RemoveNestedField(current.Object, "spec", "mycnf")
	} else if err := unstructured.SetNestedField(current.Object, value, "spec", "mycnf"); err != nil {
		return err
	}
	containers, _, err := unstructured.NestedSlice(current.Object, "spec", "podSpec", "containers")
	if err != nil {
		return err
	}
	kept := make([]any, 0, len(containers)+1)
	var mysqlContainer map[string]any
	for _, container := range containers {
		entry, ok := container.(map[string]any)
		if ok && entry["name"] == "mysql" {
			mysqlContainer = entry
			continue
		}
		kept = append(kept, container)
	}
	if cpu != "" && memory != "" {
		if mysqlContainer == nil {
			mysqlContainer = map[string]any{"name": "mysql"}
		}
		resources, _ := mysqlContainer["resources"].(map[string]any)
		if resources == nil {
			resources = map[string]any{}
		}
		for _, key := range []string{"requests", "limits"} {
			values, _ := resources[key].(map[string]any)
			if values == nil {
				values = map[string]any{}
			}
			values["cpu"], values["memory"] = cpu, memory
			resources[key] = values
		}
		mysqlContainer["resources"] = resources
	} else if mysqlContainer != nil {
		if resources, ok := mysqlContainer["resources"].(map[string]any); ok {
			for _, key := range []string{"requests", "limits"} {
				if values, ok := resources[key].(map[string]any); ok {
					delete(values, "cpu")
					delete(values, "memory")
					if len(values) == 0 {
						delete(resources, key)
					}
				}
			}
			if len(resources) == 0 {
				delete(mysqlContainer, "resources")
			}
		}
	}
	if len(mysqlContainer) > 1 {
		kept = append(kept, mysqlContainer)
	}
	if len(kept) == 0 {
		unstructured.RemoveNestedField(current.Object, "spec", "podSpec", "containers")
	} else if err := unstructured.SetNestedSlice(current.Object, kept, "spec", "podSpec", "containers"); err != nil {
		return err
	}
	updated, err := resource.Update(ctx, current, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("更新 Operator MySQL 配置: %w", err)
	}
	if cpu != "" && memory != "" && !mysqlResourceLimitsMatch(updated, cpu, memory) {
		return fmt.Errorf("Operator 未保留 MySQL 资源上限，请检查 InnoDBCluster CRD 是否支持 podSpec.containers")
	}
	return nil
}

func mysqlResourceLimitsMatch(cluster *unstructured.Unstructured, cpu, memory string) bool {
	containers, _, err := unstructured.NestedSlice(cluster.Object, "spec", "podSpec", "containers")
	if err != nil {
		return false
	}
	for _, raw := range containers {
		container, ok := raw.(map[string]any)
		if !ok || container["name"] != "mysql" {
			continue
		}
		limits, _, err := unstructured.NestedStringMap(container, "resources", "limits")
		return err == nil && limits["cpu"] == cpu && limits["memory"] == memory
	}
	return false
}

func RestartMySQLForConfig(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, value string) error {
	if item.CPULimit != "" && item.MemoryLimit != "" {
		cluster, err := client.DynamicClient.Resource(innoDBClusterGVR).Namespace(item.Namespace).Get(ctx, item.ResourceName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("读取 Operator 资源配置: %w", err)
		}
		if !mysqlResourceLimitsMatch(cluster, item.CPULimit, item.MemoryLimit) {
			return fmt.Errorf("Operator 未保存 MySQL 资源上限，请刷新配置并检查集群资源")
		}
	}
	sets := client.Clientset.AppsV1().StatefulSets(item.Namespace)
	sts, err := sets.Get(ctx, item.ResourceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("读取 MySQL 工作负载: %w", err)
	}
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas < 1 {
		return fmt.Errorf("MySQL 工作负载没有运行中的实例")
	}
	if sts.Spec.UpdateStrategy.Type == appsv1.OnDeleteStatefulSetStrategyType {
		return fmt.Errorf("MySQL 工作负载使用 OnDelete 更新策略，无法自动滚动重启")
	}
	index := -1
	for i := range sts.Spec.Template.Spec.Containers {
		if sts.Spec.Template.Spec.Containers[i].Name == "mysql" {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("MySQL 工作负载缺少 mysql 容器")
	}
	if sts.Spec.Template.Annotations == nil {
		sts.Spec.Template.Annotations = map[string]string{}
	}
	container := &sts.Spec.Template.Spec.Containers[index]
	if item.CPULimit != "" && item.MemoryLimit != "" {
		if container.Resources.Requests == nil {
			container.Resources.Requests = corev1.ResourceList{}
		}
		if container.Resources.Limits == nil {
			container.Resources.Limits = corev1.ResourceList{}
		}
		cpu, err := resource.ParseQuantity(item.CPULimit)
		if err != nil {
			return fmt.Errorf("MySQL CPU 上限无效: %w", err)
		}
		memory, err := resource.ParseQuantity(item.MemoryLimit)
		if err != nil {
			return fmt.Errorf("MySQL 内存上限无效: %w", err)
		}
		container.Resources.Requests[corev1.ResourceCPU], container.Resources.Limits[corev1.ResourceCPU] = cpu, cpu
		container.Resources.Requests[corev1.ResourceMemory], container.Resources.Limits[corev1.ResourceMemory] = memory, memory
		sts.Spec.Template.Annotations[mysqlResourceManagedAnnotation] = "true"
	} else if sts.Spec.Template.Annotations[mysqlResourceManagedAnnotation] == "true" {
		delete(container.Resources.Requests, corev1.ResourceCPU)
		delete(container.Resources.Requests, corev1.ResourceMemory)
		delete(container.Resources.Limits, corev1.ResourceCPU)
		delete(container.Resources.Limits, corev1.ResourceMemory)
		delete(sts.Spec.Template.Annotations, mysqlResourceManagedAnnotation)
	}
	if err := syncMySQLInitConfig(ctx, client, item, sts, value); err != nil {
		return err
	}
	sts.Spec.Template.Annotations[MyCNFRestartAnnotation] = MyCNFRevision(value)
	sts.Spec.Template.Annotations["cylism.io/mysql-config-restarted-at"] = time.Now().UTC().Format(time.RFC3339Nano)
	if sts.Spec.UpdateStrategy.RollingUpdate == nil {
		sts.Spec.UpdateStrategy.RollingUpdate = &appsv1.RollingUpdateStatefulSetStrategy{}
	}
	zero := int32(0)
	sts.Spec.UpdateStrategy.RollingUpdate.Partition = &zero
	_, err = sets.Update(ctx, sts, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("提交 MySQL 滚动重启: %w", err)
	}
	return nil
}

func syncMySQLInitConfig(ctx context.Context, client *k8s.Client, item *model.MySQLCluster, sts *appsv1.StatefulSet, value string) error {
	name := ""
	for _, volume := range sts.Spec.Template.Spec.Volumes {
		if volume.Name == "initconfdir" && volume.ConfigMap != nil {
			name = volume.ConfigMap.Name
			break
		}
	}
	if name == "" {
		return fmt.Errorf("MySQL 工作负载缺少初始化配置卷")
	}
	configMaps := client.Clientset.CoreV1().ConfigMaps(item.Namespace)
	configMap, err := configMaps.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("读取 MySQL 初始化配置: %w", err)
	}
	owned := false
	for _, owner := range configMap.OwnerReferences {
		if owner.Kind == "InnoDBCluster" && owner.Name == item.ResourceName {
			owned = true
			break
		}
	}
	if !owned || configMap.Data == nil {
		return fmt.Errorf("MySQL 初始化配置不属于该集群")
	}
	if _, exists := configMap.Data["99-extra.cnf"]; !exists {
		return fmt.Errorf("MySQL 初始化配置缺少 99-extra.cnf")
	}
	content := mysqlExtraConfigHeader + value
	if configMap.Data["99-extra.cnf"] == content {
		return nil
	}
	configMap.Data["99-extra.cnf"] = content
	if _, err := configMaps.Update(ctx, configMap, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("同步 MySQL 初始化配置: %w", err)
	}
	return nil
}
