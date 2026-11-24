package operations

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	"github.com/cylism/cylism-manager/internal/service/database"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMetadataAdoptsLegacyInspectionAndSurvivesFailedRefresh(t *testing.T) {
	s, _ := testService(t)
	item := model.DatabaseInstance{Name: "replica", Mode: "managed", Host: "replica", Port: 3306}
	mustCreate(t, s, &item)
	task := model.DatabaseOperation{Kind: "inspect", Source: Reference(item.ID), Status: "succeeded", Spec: "{}", Result: `{"version":"8.0.44","schemas":["bill"],"read_only":true,"password":"never-return"}`}
	mustCreate(t, s, &task)
	got, err := s.Metadata(item.ID)
	if err != nil || got.Metadata == nil || got.Metadata.Version != "8.0.44" || !got.Metadata.ReadOnly {
		t.Fatalf("metadata=%+v err=%v", got, err)
	}
	raw, _ := json.Marshal(got.Metadata)
	if strings.Contains(string(raw), "password") {
		t.Fatal("persisted non-allowlisted task fields")
	}
	if _, err = s.RefreshMetadata(context.Background(), item.ID); err == nil {
		t.Fatal("expected unavailable reader")
	}
	got, err = s.Metadata(item.ID)
	if err != nil || got.Metadata == nil || got.Metadata.Version != "8.0.44" {
		t.Fatalf("failed read erased snapshot: %+v %v", got, err)
	}
	var count int64
	s.store.DB().Model(&model.DatabaseOperation{}).Count(&count)
	if count != 1 {
		t.Fatal("reading metadata created tasks")
	}
}

func TestReadOnlyObservationsPersistAndDiscoverWithoutJobsOrLocks(t *testing.T) {
	s, client := testService(t)
	password, _ := security.Encrypt(s.key, "private-admin-password")
	source := model.DatabaseInstance{Name: "RDS", Mode: "external", Host: "source", Port: 3306, AdminUsername: "admin", EncryptedAdmin: password}
	target := model.DatabaseInstance{Name: "replica", Mode: "managed", Host: "target", Port: 3306, ClusterID: 1, EncryptedRoot: password}
	mustCreate(t, s, &source)
	mustCreate(t, s, &target)
	_, err := client.Clientset.CoreV1().Pods(database.Namespace).Create(context.Background(), &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: database.ResourceName(target.ID) + "-0", Namespace: database.Namespace}, Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "mysql", Ready: true}}}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	fail := false
	s.query = func(ctx context.Context, _ *k8s.Client, namespace, pod string, ep *endpoint, sql string) ([]map[string]string, error) {
		if fail {
			return nil, errors.New("unreachable")
		}
		if ep.Password != "private-admin-password" {
			t.Fatal("did not use saved credentials")
		}
		if sql == metadataSQL {
			uuid, readonly := "source-uuid", 0
			if ep.Host == "target" {
				uuid, readonly = "target-uuid", 1
			}
			raw, _ := json.Marshal(map[string]any{"version": "8.0.44", "server_uuid": uuid, "read_only": readonly, "schemas": []string{"bill", "account"}})
			return []map[string]string{{"metadata": string(raw)}}, nil
		}
		if sql != "SHOW REPLICA STATUS" {
			t.Fatalf("unexpected SQL: %s", sql)
		}
		return []map[string]string{{"Source_UUID": "source-uuid", "Channel_Name": "existing", "Replica_IO_Running": "Yes", "Replica_SQL_Running": "No", "Seconds_Behind_Source": "NULL", "Last_IO_Errno": "0", "Last_SQL_Errno": "1062", "Source_Password": "never-return"}}, nil
	}
	item, err := s.RefreshMetadata(context.Background(), target.ID)
	if err != nil || item.Metadata == nil || !item.Metadata.ReadOnly {
		t.Fatalf("refresh=%+v %v", item, err)
	}
	// A restarted service reads the same persisted metadata without a live query.
	restarted := New(s.store, s.key, nil)
	cached, err := restarted.Metadata(target.ID)
	if err != nil || cached.Metadata == nil || cached.Metadata.Schemas[0] != "account" {
		t.Fatalf("persisted=%+v %v", cached, err)
	}
	relations, err := s.RefreshReplications(context.Background(), target.ID)
	if err != nil || len(relations) != 1 {
		t.Fatalf("relations=%+v %v", relations, err)
	}
	relation := relations[0]
	if relation.Source != Reference(source.ID) || relation.Channel != "existing" || relation.Status != "error" || relation.SQLRunning || relation.LagSeconds != nil || relation.LastCheckedAt == nil {
		t.Fatalf("incorrect status: %+v", relation)
	}
	if strings.Contains(relation.Observation, "never-return") {
		t.Fatal("leaked replication credentials")
	}
	fail = true
	if _, err = s.RefreshReplications(context.Background(), target.ID); err == nil {
		t.Fatal("expected read failure")
	}
	var stored model.DatabaseReplication
	s.store.DB().First(&stored, relation.ID)
	if !stored.LastCheckedAt.Equal(*relation.LastCheckedAt) {
		t.Fatal("failed query replaced last observation")
	}
	for _, table := range []any{&model.DatabaseOperation{}, &model.DatabaseOperationLock{}} {
		var count int64
		s.store.DB().Model(table).Count(&count)
		if count != 0 {
			t.Fatal("read-only query created task or lock")
		}
	}
	jobs, _ := client.Clientset.BatchV1().Jobs(database.Namespace).List(context.Background(), metav1.ListOptions{})
	if len(jobs.Items) != 0 {
		t.Fatal("read-only query created a Job")
	}
}

func TestParseBatchRowsPreservesJSONEscapesAndRejectsMalformedRows(t *testing.T) {
	rows, err := parseMySQLRows("metadata\n{\"version\":\"8.0.44\",\"schemas\":[\"db\"]}\n")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	var value map[string]any
	if json.Unmarshal([]byte(rows[0]["metadata"]), &value) != nil {
		t.Fatal("invalid JSON response")
	}
	if _, err := parseMySQLRows("a\tb\nmissing\n"); err == nil {
		t.Fatal("accepted malformed row")
	}
}
