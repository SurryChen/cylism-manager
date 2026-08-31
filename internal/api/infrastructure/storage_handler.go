package infrastructure

import (
	"net/http"
	"strconv"
	"strings"

	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
	"github.com/cylism/cylism-manager/internal/service/storage"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// StorageHandler is the infrastructure HTTP boundary for PVC endpoints. The
// underlying workflows are injected so long-running migration/import jobs can
// retain their existing recovery behaviour while they are progressively moved
// into the storage service.
type StorageHandler struct {
	store   repository.PVCRepository
	k8s     *k8sclient.Client
	encKey  []byte
	Service *storage.Service
}

func NewStorageHandler(service *storage.Service, h StorageHandler, clients ...*k8sclient.Client) *StorageHandler {
	h.Service = service
	if len(clients) > 0 {
		h.k8s = clients[0]
	}
	return &h
}

func NewStorageHandlerWithClient(service *storage.Service, st repository.PVCRepository, encKey []byte, client *k8sclient.Client) *StorageHandler {
	return &StorageHandler{Service: service, store: st, encKey: encKey, k8s: client}
}

func (h *StorageHandler) ConfigureStorageExecutor() {
	if h.Service == nil {
		return
	}
	h.Service.WithAsyncExecutor(storage.AsyncExecutor{
		RunMigration: h.runPersistentVolumeMigration,
		RunImport:    h.runHostDirectoryPVCImport,
		RunBackup:    h.executePersistentVolumeBackup,
		RunRestore:   h.executePersistentVolumeRestore,
	})
}

func storageK8sUnavailable(c *gin.Context) {
	model.Error(c, http.StatusOK, model.CodeK8sUnavailable, "K8s 集群未连接")
}

func optionalStorageQueryID(c *gin.Context, key string) (uint, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	return uint(id), err
}

func parseStorageID(value string) (uint, error) {
	id, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	return uint(id), err
}

func storageRequestUserID(c *gin.Context) uint {
	if value, ok := c.Get("user_id"); ok {
		switch id := value.(type) {
		case uint:
			return id
		case uint64:
			return uint(id)
		case int:
			return uint(id)
		}
	}
	return 0
}

func storageErrorsIsNotFound(err error) bool { return err == gorm.ErrRecordNotFound }
