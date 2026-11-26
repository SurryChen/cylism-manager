package store

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func TestProductTaskGroupOwnershipAndRename(t *testing.T) {
	s := setupTestDB(t)
	product := lifecycleProduct(t, s, 1, "board")
	other := lifecycleProduct(t, s, 1, "another board")
	group := model.ProductTaskGroup{ProductID: product.ID, Name: "发布验证"}
	if err := s.SaveProductTaskGroup(1, &group); err != nil {
		t.Fatal(err)
	}
	duplicate := model.ProductTaskGroup{ProductID: product.ID, Name: group.Name}
	if err := s.SaveProductTaskGroup(1, &duplicate); !errors.Is(err, model.ErrProductTaskGroupName) {
		t.Fatalf("duplicate name: %v", err)
	}
	duplicate.ProductID = other.ID
	if err := s.SaveProductTaskGroup(1, &duplicate); err != nil {
		t.Fatalf("same name in another project: %v", err)
	}
	if _, err := s.ProductTaskGroups(2, product.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("foreign group listing: %v", err)
	}
	group.Name = "上线验收"
	if err := s.SaveProductTaskGroup(2, &group); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("foreign group update: %v", err)
	}
	if err := s.SaveProductTaskGroup(1, &group); err != nil {
		t.Fatal(err)
	}
	groups, err := s.ProductTaskGroups(1, product.ID)
	if err != nil || len(groups) != 1 || groups[0].ID != group.ID || groups[0].Name != group.Name {
		t.Fatalf("renamed groups: %+v, %v", groups, err)
	}
	group.ProductID = other.ID
	if err := s.SaveProductTaskGroup(1, &group); !errors.Is(err, model.ErrProductResourceAccess) {
		t.Fatalf("cross-project group update: %v", err)
	}
}

func TestProductTaskGroupDeletionPreservesTasksAndVersionLinks(t *testing.T) {
	s := setupTestDB(t)
	product := lifecycleProduct(t, s, 1, "board")
	group := model.ProductTaskGroup{ProductID: product.ID, Name: "发布验证"}
	if err := s.SaveProductTaskGroup(1, &group); err != nil {
		t.Fatal(err)
	}
	version := model.ProductVersion{ProductID: product.ID, Name: "1.0"}
	if err := s.SaveProductVersion(1, &version); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"todo", "done", "cancelled"} {
		task := model.ProductTask{ProductID: product.ID, GroupID: group.ID, Title: status, Kind: "action", Status: status, Priority: "normal", Details: "保留说明", VersionIDs: []uint{version.ID}}
		if err := s.SaveProductTask(1, &task); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DeleteProductTaskGroup(2, product.ID, group.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("foreign group deletion: %v", err)
	}
	if err := s.DeleteProductTaskGroup(1, product.ID, group.ID); err != nil {
		t.Fatal(err)
	}
	groupID := uint(0)
	rows, count, err := s.ListProductTasks(1, product.ID, model.ProductLifecycleFilter{TaskGroupID: &groupID, Limit: 10})
	if err != nil || count != 3 || len(rows) != 3 {
		t.Fatalf("ungrouped tasks: %+v, %d, %v", rows, count, err)
	}
	for _, task := range rows {
		if task.GroupID != 0 || task.Revision != 2 || task.Details != "保留说明" || len(task.VersionIDs) != 1 || task.VersionIDs[0] != version.ID || task.Title != task.Status {
			t.Fatalf("task lost data: %+v", task)
		}
		stale := task
		stale.Revision--
		stale.GroupID = group.ID
		if err := s.SaveProductTask(1, &stale); !errors.Is(err, model.ErrProductLifecycleConflict) {
			t.Fatalf("stale task restored deleted group: %v", err)
		}
	}
	groups, err := s.ProductTaskGroups(1, product.ID)
	if err != nil || len(groups) != 0 {
		t.Fatalf("deleted group still listed: %+v, %v", groups, err)
	}
}

func TestProductTaskGroupDeletionRollsBackOnTaskUpdateFailure(t *testing.T) {
	s := setupTestDB(t)
	product := lifecycleProduct(t, s, 1, "board")
	group := model.ProductTaskGroup{ProductID: product.ID, Name: "发布验证"}
	if err := s.SaveProductTaskGroup(1, &group); err != nil {
		t.Fatal(err)
	}
	task := model.ProductTask{ProductID: product.ID, GroupID: group.ID, Title: "验证", Kind: "action", Status: "todo", Priority: "normal"}
	if err := s.SaveProductTask(1, &task); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Exec("CREATE TRIGGER fail_task_group_reset BEFORE UPDATE OF group_id ON product_tasks BEGIN SELECT RAISE(ABORT, 'forced group reset failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProductTaskGroup(1, product.ID, group.ID); err == nil {
		t.Fatal("expected failed task update to abort deletion")
	}
	groups, err := s.ProductTaskGroups(1, product.ID)
	if err != nil || len(groups) != 1 || groups[0].ID != group.ID {
		t.Fatalf("group deletion was not rolled back: %+v, %v", groups, err)
	}
	var saved model.ProductTask
	if err := s.db.First(&saved, task.ID).Error; err != nil || saved.GroupID != group.ID || saved.Revision != task.Revision {
		t.Fatalf("task changed after failed deletion: %+v, %v", saved, err)
	}
}

func TestProductTaskGroupMigrationPreservesLegacyTasks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-task-groups.db")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	db, err := s.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	product := lifecycleProduct(t, s, 1, "legacy")
	task := model.ProductTask{ProductID: product.ID, Title: "已有待办", Kind: "action", Status: "todo", Priority: "normal"}
	if err := s.SaveProductTask(1, &task); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Migrator().DropColumn(&model.ProductTask{}, "GroupID"); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Migrator().DropTable(&model.ProductTaskGroup{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		upgraded, err := New(path)
		if err != nil {
			t.Fatal(err)
		}
		upgradedDB, err := upgraded.db.DB()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = upgradedDB.Close() })
		rows, count, err := upgraded.ListProductTasks(1, product.ID, model.ProductLifecycleFilter{Limit: 10})
		if err != nil || count != 1 || len(rows) != 1 || rows[0].ID != task.ID || rows[0].GroupID != 0 || rows[0].Revision != task.Revision || rows[0].Title != task.Title {
			t.Fatalf("legacy task migration: %+v, %d, %v", rows, count, err)
		}
		groups, err := upgraded.ProductTaskGroups(1, product.ID)
		if err != nil || groups == nil || len(groups) != 0 {
			t.Fatalf("new group storage: %+v, %v", groups, err)
		}
		if err := upgradedDB.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
