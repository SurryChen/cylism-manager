package operations

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/service/database"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

const metadataSQL = `SELECT JSON_OBJECT('version', VERSION(), 'server_uuid', @@server_uuid, 'read_only', @@global.read_only, 'schemas', (SELECT JSON_ARRAYAGG(SCHEMA_NAME) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME NOT IN ('mysql','sys','information_schema','performance_schema'))) AS metadata`

// Old successful inspections also become reusable instance information. Never
// serialize arbitrary task output or turn merely opening details into a Job.
func saveInspectionMetadata(tx *gorm.DB, task *model.DatabaseOperation) error {
	_, id, err := ParseReference(task.Source)
	if err != nil || !strings.HasPrefix(task.Source, "instance:") {
		return nil
	}
	var metadata model.DatabaseMetadata
	if json.Unmarshal([]byte(task.Result), &metadata) != nil || metadata.Version == "" {
		return nil
	}
	metadata.CheckedAt = task.UpdatedAt
	if metadata.CheckedAt.IsZero() {
		metadata.CheckedAt = time.Now()
	}
	var item model.DatabaseInstance
	if err := tx.First(&item, id).Error; err != nil {
		return err
	}
	if item.Metadata != nil && !item.Metadata.CheckedAt.Before(metadata.CheckedAt) {
		return nil
	}
	item.Metadata = &metadata
	return tx.Model(&item).Select("Metadata").Updates(&item).Error
}

func (s *Service) Metadata(id uint) (*model.DatabaseInstance, error) {
	var item model.DatabaseInstance
	if err := s.store.DB().First(&item, id).Error; err != nil {
		return nil, err
	}
	if item.Metadata == nil {
		var task model.DatabaseOperation
		if err := s.store.DB().Where("source = ? AND kind = ? AND status = ?", Reference(id), "inspect", "succeeded").Order("id desc").Limit(1).Find(&task).Error; err != nil {
			return nil, err
		}
		if task.ID != 0 {
			if err := saveInspectionMetadata(s.store.DB(), &task); err != nil {
				return nil, err
			}
			if err := s.store.DB().First(&item, id).Error; err != nil {
				return nil, err
			}
		}
	}
	return &item, nil
}

func quoteArgument(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

// Reuse an existing MySQL client in a running database pod. No operator,
// temporary Job, migration lock, or credential-bearing Secret is required.
func queryMySQLPod(ctx context.Context, client *k8s.Client, namespace, pod string, ep *endpoint, sql string) ([]map[string]string, error) {
	if client == nil || client.Config == nil || client.Clientset == nil || client.Clientset.CoreV1().RESTClient() == nil {
		return nil, fmt.Errorf("数据库只读查询通道不可用")
	}
	if strings.ContainsAny(ep.Password, "\r\n") {
		return nil, fmt.Errorf("数据库凭据格式不支持只读查询")
	}
	args := []string{"mysql", "--protocol=tcp", "--ssl-mode=REQUIRED", "--connect-timeout=5", "--batch", "--host=" + ep.Host, "--port=" + strconv.Itoa(ep.Port), "--user=" + ep.User, "--execute=" + sql}
	for i := range args {
		args[i] = quoteArgument(args[i])
	}
	command := "IFS= read -r MYSQL_PWD || exit 1; export MYSQL_PWD; exec " + strings.Join(args, " ")
	request := client.Clientset.CoreV1().RESTClient().Post().Resource("pods").Namespace(namespace).Name(pod).SubResource("exec").VersionedParams(&corev1.PodExecOptions{Container: "mysql", Command: []string{"/bin/sh", "-c", command}, Stdin: true, Stdout: true, Stderr: true}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(client.Config, http.MethodPost, request.URL())
	if err != nil {
		return nil, fmt.Errorf("建立只读查询通道失败")
	}
	var stdout, stderr bytes.Buffer
	if err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdin: strings.NewReader(ep.Password + "\n"), Stdout: &stdout, Stderr: &stderr}); err != nil {
		return nil, fmt.Errorf("只读查询失败，请检查所选实例的管理凭据、网络及查询权限")
	}
	// Batch output escapes embedded tabs and line breaks; never return stderr,
	// which may include sensitive connection information.
	if stdout.Len() > 1024*1024 {
		return nil, fmt.Errorf("查询结果过大")
	}
	return parseMySQLRows(stdout.String())
}

func parseMySQLRows(output string) ([]map[string]string, error) {
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	rows := []map[string]string{}
	if len(lines) == 1 && lines[0] == "" {
		return rows, nil
	}
	columns := strings.Split(strings.TrimSuffix(lines[0], "\r"), "\t")
	for _, line := range lines[1:] {
		values := strings.Split(strings.TrimSuffix(line, "\r"), "\t")
		if len(values) != len(columns) {
			return nil, fmt.Errorf("数据库查询响应格式无效")
		}
		row := map[string]string{}
		for i, key := range columns {
			row[key] = strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\r`, "\r", `\\`, `\`).Replace(values[i])
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (s *Service) reader(ctx context.Context, item *model.DatabaseInstance) (*k8s.Client, string, string, uint, error) {
	clusterID := item.ClusterID
	if item.Mode != "managed" {
		var gw model.DatabaseGateway
		if err := s.store.DB().Where("instance_id = ?", item.ID).First(&gw).Error; err == nil {
			clusterID = gw.ClusterID
		}
	}
	var candidates []model.DatabaseInstance
	query := s.store.DB().Where("mode = ?", "managed")
	if clusterID != 0 {
		query = query.Where("cluster_id = ?", clusterID)
	}
	if err := query.Order("id").Find(&candidates).Error; err != nil {
		return nil, "", "", 0, err
	}
	// Prefer the instance's own container, then another known managed MySQL.
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].ID == item.ID && candidates[j].ID != item.ID })
	for _, candidate := range candidates {
		client, err := s.client(ctx, candidate.ClusterID)
		if err != nil || client == nil || client.Clientset == nil {
			continue
		}
		namespace := candidate.Namespace
		if namespace == "" {
			namespace = database.Namespace
		}
		name := candidate.ResourceName
		if name == "" {
			name = database.ResourceName(candidate.ID)
		}
		pod, err := client.Clientset.CoreV1().Pods(namespace).Get(ctx, name+"-0", metav1.GetOptions{})
		if err != nil || pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, container := range pod.Status.ContainerStatuses {
			if container.Name == "mysql" && container.Ready {
				return client, namespace, pod.Name, candidate.ClusterID, nil
			}
		}
	}
	return nil, "", "", 0, fmt.Errorf("当前没有可复用的就绪 MySQL 客户端；请先恢复独立 MySQL 实例")
}

func (s *Service) observe(ctx context.Context, id uint, channels bool) (*model.DatabaseInstance, []map[string]string, error) {
	item, err := s.Metadata(id)
	if err != nil {
		return nil, nil, err
	}
	if s.client == nil {
		return nil, nil, fmt.Errorf("集群客户端未配置")
	}
	client, namespace, pod, clusterID, err := s.reader(ctx, item)
	if err != nil {
		return nil, nil, err
	}
	ep, err := s.resolve(ctx, s.store.DB(), Reference(id), clusterID)
	if err != nil {
		return nil, nil, err
	}
	rows, err := s.query(ctx, client, namespace, pod, ep, metadataSQL)
	if err != nil {
		return nil, nil, err
	}
	var metadata model.DatabaseMetadata
	// MySQL JSON encodes boolean system variables as 0/1.
	var raw struct {
		Version  string   `json:"version"`
		UUID     string   `json:"server_uuid"`
		ReadOnly int      `json:"read_only"`
		Schemas  []string `json:"schemas"`
	}
	if len(rows) != 1 || json.Unmarshal([]byte(rows[0]["metadata"]), &raw) != nil || raw.Version == "" || raw.UUID == "" {
		return nil, nil, fmt.Errorf("实例信息响应无效")
	}
	metadata = model.DatabaseMetadata{Version: raw.Version, ServerUUID: raw.UUID, ReadOnly: raw.ReadOnly != 0, Schemas: raw.Schemas, CheckedAt: time.Now()}
	if metadata.Schemas == nil {
		metadata.Schemas = []string{}
	}
	sort.Strings(metadata.Schemas)
	item.Metadata = &metadata
	// Update through the model so GORM's JSON serializer is used.
	if err = s.store.DB().Model(item).Select("Metadata").Updates(item).Error; err != nil {
		return nil, nil, err
	}
	if !channels {
		return item, nil, nil
	}
	rows, err = s.query(ctx, client, namespace, pod, ep, "SHOW REPLICA STATUS")
	return item, rows, err
}

func (s *Service) RefreshMetadata(ctx context.Context, id uint) (*model.DatabaseInstance, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	item, _, err := s.observe(ctx, id, false)
	return item, err
}

// Discover current upstreams by server UUID. Viewing or refreshing a relation
// never starts replication, changes its filters, or creates a database task.
func (s *Service) RefreshReplications(ctx context.Context, id uint) ([]model.DatabaseReplication, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	selected, err := s.Metadata(id)
	if err != nil {
		return nil, err
	}
	clusterID := selected.ClusterID
	if selected.Mode == "external" {
		var gw model.DatabaseGateway
		if err := s.store.DB().Where("instance_id = ?", id).First(&gw).Error; err == nil {
			clusterID = gw.ClusterID
		}
	}
	var saved []model.DatabaseReplication
	if err := s.store.DB().Where("source = ? OR target = ?", Reference(id), Reference(id)).Find(&saved).Error; err != nil {
		return nil, err
	}
	knownTargets := map[string]bool{}
	for _, relation := range saved {
		knownTargets[relation.Target] = true
	}
	var instances []model.DatabaseInstance
	if err := s.store.DB().Order("id").Find(&instances).Error; err != nil {
		return nil, err
	}
	warnings := []string{}
	refreshed := 0
	for _, target := range instances {
		// External sources are queried for identity, managed instances for channels.
		known := knownTargets[Reference(target.ID)]
		candidate := clusterID != 0 && target.ClusterID == clusterID
		if target.Mode != "managed" || (target.ID != id && !known && !candidate) {
			continue
		}
		observed, rows, err := s.observe(ctx, target.ID, true)
		if err != nil {
			if target.ID == id || known {
				return nil, err
			}
			warnings = append(warnings, fmt.Sprintf("%s：未能读取复制通道", target.Name))
			continue
		}
		refreshed++
		if len(rows) > 1 {
			return nil, fmt.Errorf("检测到多源复制，当前不自动登记，请保留现有通道")
		}
		for _, row := range rows {
			sourceRef := ""
			for _, source := range instances {
				if source.ID == target.ID {
					continue
				}
				candidate := &source
				if candidate.Metadata == nil || candidate.Metadata.ServerUUID == "" || strings.EqualFold(candidate.Metadata.ServerUUID, row["Source_UUID"]) {
					candidate, _, err = s.observe(ctx, source.ID, false)
					if err != nil {
						continue
					}
				}
				if strings.EqualFold(candidate.Metadata.ServerUUID, row["Source_UUID"]) {
					sourceRef = Reference(source.ID)
					break
				}
			}
			if sourceRef == "" {
				if target.ID == id {
					warnings = append(warnings, "复制源尚未接入平台或无法核对源 UUID")
				}
				continue
			}
			if target.ID != id && sourceRef != Reference(id) {
				continue
			}
			lag, err := strconv.ParseInt(row["Seconds_Behind_Source"], 10, 64)
			var lagSeconds *int64
			if err == nil {
				lagSeconds = &lag
			}
			ioErr, _ := strconv.Atoi(row["Last_IO_Errno"])
			sqlErr, _ := strconv.Atoi(row["Last_SQL_Errno"])
			observation := replicationObservation{Channel: row["Channel_Name"], SourceVerified: true, IORunning: row["Replica_IO_Running"] == "Yes", SQLRunning: row["Replica_SQL_Running"] == "Yes", LagSeconds: lagSeconds, IOErrno: ioErr, SQLErrno: sqlErr, ReadOnly: observed.Metadata.ReadOnly, Schemas: row["Replicate_Do_DB"], Tables: row["Replicate_Wild_Do_Table"]}
			raw, _ := json.Marshal(observation)
			task := model.DatabaseOperation{Kind: "replication_discover", Source: sourceRef, Target: Reference(target.ID)}
			if err = s.store.DB().Transaction(func(tx *gorm.DB) error { return recordReplicationObservation(tx, &task, string(raw)) }); err != nil {
				return nil, err
			}
		}
		if len(rows) == 0 {
			if err := s.store.DB().Model(&model.DatabaseReplication{}).Where("target = ? AND (source = ? OR target = ?)", Reference(target.ID), Reference(id), Reference(id)).Updates(map[string]any{"status": "missing", "last_checked_at": time.Now(), "io_running": false, "sql_running": false, "lag_seconds": nil}).Error; err != nil {
				return nil, err
			}
		}
	}
	if len(warnings) > 0 && refreshed == 0 {
		return nil, fmt.Errorf("%s", strings.Join(warnings, "；"))
	}
	relations := []model.DatabaseReplication{}
	err = s.store.DB().Where("source = ? OR target = ?", Reference(id), Reference(id)).Order("id desc").Find(&relations).Error
	return relations, err
}
