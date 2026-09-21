package storage

import (
	"context"
	"errors"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/transport"
	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/api/resource"
)

type persistentVolumeClaimUsageResponse struct {
	Namespace     string `json:"namespace"`
	Name          string `json:"name"`
	UsedBytes     int64  `json:"used_bytes,omitempty"`
	CapacityBytes int64  `json:"capacity_bytes,omitempty"`
	Status        string `json:"status"`
	Message       string `json:"message,omitempty"`
}

var ReadPersistentVolumeUsage = readLocalPersistentVolumeUsage

// ListPersistentVolumeClaimUsage reads local-path directory usage separately
// from the inventory so a slow or unreachable node never delays the storage UI.
func (h *StorageHandler) ListPersistentVolumeClaimUsage(c *gin.Context) {
	if h.pvc == nil {
		storageK8sUnavailable(c)
		return
	}
	claims, err := h.pvc.ListPVCsContext(c.Request.Context(), strings.TrimSpace(c.Query("namespace")))
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}

	serversByNode := map[string]model.Server{}
	if h.store != nil {
		if servers, listErr := h.store.ListServers(); listErr == nil {
			for _, server := range servers {
				if nodeName := strings.TrimSpace(server.K8sNodeName); nodeName != "" {
					serversByNode[nodeName] = server
				}
			}
		}
	}

	responses := make([]persistentVolumeClaimUsageResponse, len(claims))
	type usageJob struct {
		index  int
		claim  k8sclient.PersistentVolumeClaimInfo
		server model.Server
		lock   *sync.Mutex
	}
	jobs := make([]usageJob, 0, len(claims))
	nodeLocks := map[string]*sync.Mutex{}
	for index := range claims {
		claim := claims[index]
		response := persistentVolumeClaimUsageResponse{Namespace: claim.Namespace, Name: claim.Name, CapacityBytes: pvcCapacityBytes(claim.Storage), Status: "unavailable"}
		server, found := serversByNode[claim.BoundNode]
		switch {
		case claim.Phase != "Bound":
			response.Message = "存储卷尚未绑定"
		case !claim.Managed:
			response.Message = "仅平台托管卷支持采集"
		case !claim.IsLocal || claim.LocalPath == "":
			response.Message = "当前存储类型暂不支持采集"
		case !persistentVolumeUsagePathAllowed(claim.LocalPath):
			response.Message = "当前本地目录不支持采集"
		case claim.BoundNode == "":
			response.Message = "未识别存储卷绑定节点"
		case !found:
			response.Message = "绑定节点未登记 SSH"
		default:
			lock := nodeLocks[claim.BoundNode]
			if lock == nil {
				lock = &sync.Mutex{}
				nodeLocks[claim.BoundNode] = lock
			}
			jobs = append(jobs, usageJob{index: index, claim: claim, server: server, lock: lock})
		}
		responses[index] = response
	}

	workerCount := 4
	if len(jobs) < workerCount {
		workerCount = len(jobs)
	}
	jobQueue := make(chan usageJob)
	var waitGroup sync.WaitGroup
	for range workerCount {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for job := range jobQueue {
				job.lock.Lock()
				usedBytes, readErr := ReadPersistentVolumeUsage(c.Request.Context(), &job.server, job.claim.LocalPath, h.encKey)
				job.lock.Unlock()
				if readErr != nil {
					responses[job.index].Message = "节点不可访问或存储目录不可读取"
					continue
				}
				responses[job.index].UsedBytes = usedBytes
				responses[job.index].Status = "available"
			}
		}()
	}
	for _, job := range jobs {
		jobQueue <- job
	}
	close(jobQueue)
	waitGroup.Wait()

	apiShared.Success(c, responses)
}

func readLocalPersistentVolumeUsage(ctx context.Context, server *model.Server, localPath string, encKey []byte) (int64, error) {
	command := "sudo -n du -sb -- " + storageShellQuote(localPath)
	output, err := transport.SSHExecServerContext(ctx, 12*time.Second, server, encKey, command)
	if err != nil {
		return 0, err
	}
	return parsePersistentVolumeUsage(output)
}

func parsePersistentVolumeUsage(output []byte) (int64, error) {
	lines := strings.Split(string(output), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		fields := strings.Fields(lines[index])
		if len(fields) == 0 {
			continue
		}
		usedBytes, err := strconv.ParseInt(fields[0], 10, 64)
		if err == nil && usedBytes >= 0 {
			return usedBytes, nil
		}
	}
	return 0, errors.New("未读取到目录用量")
}

func ParsePersistentVolumeUsage(output []byte) (int64, error) {
	return parsePersistentVolumeUsage(output)
}

func pvcCapacityBytes(storage string) int64 {
	quantity, err := resource.ParseQuantity(strings.TrimSpace(storage))
	if err != nil || quantity.Sign() <= 0 {
		return 0
	}
	return quantity.Value()
}

func persistentVolumeUsagePathAllowed(localPath string) bool {
	const k3sLocalPathRoot = "/var/lib/rancher/k3s/storage"
	cleaned := path.Clean(localPath)
	return strings.HasPrefix(cleaned, k3sLocalPathRoot+"/")
}
