package cloud

import (
	"context"
	"errors"
	"fmt"
	"github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cylism/cylism-manager/internal/service/database"
)

func TestSwitchTargetRequiresReadySecondary(t *testing.T) {
	pods := []database.ClusterPod{
		{Name: "mysql-0", Component: "mysqld", DatabaseRole: "PRIMARY", NodeName: "node-a", Ready: true},
		{Name: "mysql-1", Component: "mysqld", DatabaseRole: "SECONDARY", NodeName: "node-b", Ready: true},
		{Name: "mysql-2", Component: "mysqld", DatabaseRole: "SECONDARY", NodeName: "node-c", Ready: false},
		{Name: "router-0", Component: "mysqlrouter", NodeName: "node-b", Ready: true},
	}
	snapshot := &database.ClusterSnapshot{Status: "ready", Pods: pods}
	if target, err := switchTarget(snapshot, "mysql-1"); err != nil || target.NodeName != "node-b" {
		t.Fatalf("ready secondary rejected: target=%+v err=%v", target, err)
	}
	for _, name := range []string{"mysql-0", "mysql-2", "router-0", "unknown"} {
		if _, err := switchTarget(snapshot, name); err == nil {
			t.Fatalf("ineligible target %s was accepted", name)
		}
	}
	snapshot.Status = "starting"
	if _, err := switchTarget(snapshot, "mysql-1"); err == nil || !strings.Contains(err.Error(), "尚未就绪") {
		t.Fatalf("unready cluster accepted: %v", err)
	}
}

func TestPrimarySwitchReturnsBeforeClusterIOAndPersistsFailure(t *testing.T) {
	s, err := store.New("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	item := model.MySQLCluster{Name: "test", ClusterID: 1, Namespace: "mysql", ResourceName: "mysql-test", Status: "ready"}
	if err := s.DB().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	entered := make(chan context.Context, 1)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	h := NewDatabaseHandler(s, nil, func(ctx context.Context, _ uint) (*k8s.Client, error) {
		entered <- ctx
		<-release
		return nil, errors.New("connection unavailable")
	})
	r := gin.New()
	r.POST("/clusters/:id/switch", h.SwitchClusterPrimary)
	r.GET("/clusters/:id/tasks/:taskID/logs", h.MySQLClusterTaskLogs)
	r.POST("/clusters/:id/scale", h.AddMySQLInstance)
	r.POST("/clusters/:id/restart", h.RestartMySQLClusterForConfig)
	requestCtx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/clusters/%d/switch", item.ID), strings.NewReader(`{"pod_name":"mysql-test-1"}`)).WithContext(requestCtx)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	returned := make(chan struct{})
	go func() { r.ServeHTTP(response, request); close(returned) }()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP handler waited for background cluster I/O")
	}
	cancel()
	if response.Code != http.StatusAccepted {
		t.Fatalf("response: %d %s", response.Code, response.Body.String())
	}
	var workerCtx context.Context
	select {
	case workerCtx = <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("worker not started")
	}
	if workerCtx.Err() != nil {
		t.Fatal("request cancellation cancelled background worker")
	}
	// Runtime synchronization uses this lock: it must remain available while switching.
	lockValue, _ := h.mysqlSyncLocks.Load(item.ID)
	lock := lockValue.(*sync.Mutex)
	if !lock.TryLock() {
		t.Fatal("background worker blocks cluster queries")
	}
	lock.Unlock()
	var task model.MySQLClusterTask
	if err := s.DB().First(&task).Error; err != nil {
		t.Fatal(err)
	}
	if task.Status != "running" || task.Kind != "switch_primary" {
		t.Fatalf("task not persisted: %+v", task)
	}
	logs := httptest.NewRecorder()
	r.ServeHTTP(logs, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/clusters/%d/tasks/%d/logs", item.ID, task.ID), nil))
	if logs.Code != http.StatusOK || !strings.Contains(logs.Body.String(), "目标副本") {
		t.Fatalf("running logs unavailable: %s", logs.Body.String())
	}
	for _, operation := range []string{"switch", "scale", "restart"} {
		duplicate := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/clusters/%d/%s", item.ID, operation), strings.NewReader(`{"pod_name":"mysql-test-1"}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(duplicate, req)
		if duplicate.Code != http.StatusConflict {
			t.Fatalf("%s accepted during switch: %d %s", operation, duplicate.Code, duplicate.Body.String())
		}
	}
	once.Do(func() { close(release) })
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := s.DB().First(&task, task.ID).Error; err != nil {
			t.Fatal(err)
		}
		if task.Status == "failed" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("background failure was not persisted")
		case <-ticker.C:
		}
	}
	var entries []model.MySQLClusterTaskLog
	s.DB().Where("task_id = ?", task.ID).Order("id asc").Find(&entries)
	if len(entries) != 3 || !strings.Contains(entries[2].Message, "connection unavailable") {
		t.Fatalf("missing progress/failure logs: %+v", entries)
	}
}

func TestPrimarySwitchRecoveryAndTerminalResultsAreIdempotent(t *testing.T) {
	s, err := store.New("file:" + t.Name() + "?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	h := NewDatabaseHandler(s, nil, nil)
	old := model.MySQLClusterTask{ClusterID: 1, Kind: "switch_primary", Status: "running", CreatedAt: time.Now().Add(-3 * time.Minute)}
	current := model.MySQLClusterTask{ClusterID: 1, Kind: "switch_primary", Status: "running"}
	other := model.MySQLClusterTask{ClusterID: 2, Kind: "switch_primary", Status: "running", CreatedAt: old.CreatedAt}
	for _, task := range []*model.MySQLClusterTask{&old, &current, &other} {
		if err := s.DB().Create(task).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := h.expirePrimarySwitchTasks(1); err != nil {
			t.Fatal(err)
		}
	}
	if h.recordPrimarySwitchProgress(old.ID, "success", "late result") {
		t.Fatal("late worker overwrote recovery")
	}
	if !h.recordPrimarySwitchProgress(current.ID, "success", "verified primary") {
		t.Fatal("success not saved")
	}
	if h.recordPrimarySwitchProgress(current.ID, "failed", "late failure") {
		t.Fatal("terminal result overwritten")
	}
	s.DB().First(&old, old.ID)
	s.DB().First(&current, current.ID)
	s.DB().First(&other, other.ID)
	if old.Status != "failed" || !strings.Contains(old.LastProgress, "核对实际主实例") || current.Status != "success" || other.Status != "running" {
		t.Fatalf("bad recovery: %+v %+v %+v", old, current, other)
	}
	var logs []model.MySQLClusterTaskLog
	s.DB().Order("id asc").Find(&logs)
	if len(logs) != 2 {
		t.Fatalf("repeated recovery or terminal update duplicated logs: %+v", logs)
	}
}
