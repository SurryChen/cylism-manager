package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	"github.com/cylism/cylism-manager/internal/application"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/api/resource"
)

type persistentVolumeClaimRequest struct {
	EnvironmentID     uint   `json:"environment_id"`
	Namespace         string `json:"namespace"`
	Name              string `json:"name"`
	Storage           string `json:"storage"`
	StorageClassName  string `json:"storage_class_name"`
	ConfirmDataDelete bool   `json:"confirm_data_delete"`
}

type persistentVolumeClaimResponse struct {
	k8sclient.PersistentVolumeClaimInfo
	BoundNodeDisplayName string   `json:"bound_node_display_name,omitempty"`
	References           []string `json:"references,omitempty"`
	ProjectID            uint     `json:"project_id,omitempty"`
	ProjectName          string   `json:"project_name,omitempty"`
	EnvironmentName      string   `json:"environment_name,omitempty"`
}

type persistentVolumeClaimUsageResponse struct {
	Namespace     string `json:"namespace"`
	Name          string `json:"name"`
	UsedBytes     int64  `json:"used_bytes,omitempty"`
	CapacityBytes int64  `json:"capacity_bytes,omitempty"`
	Status        string `json:"status"`
	Message       string `json:"message,omitempty"`
}

var ReadPersistentVolumeUsage = readLocalPersistentVolumeUsage

func (h *StorageHandler) ListPersistentVolumeClaims(c *gin.Context) {
	if h.pvc == nil {
		storageK8sUnavailable(c)
		return
	}
	environmentID, err := apiShared.OptionalID(strings.TrimSpace(c.Query("environment_id")))
	if err != nil {
		apiShared.BadRequest(c, "环境 ID 无效")
		return
	}
	namespace := strings.TrimSpace(c.Query("namespace"))
	claims, err := h.Service.ListPVCsContext(c.Request.Context(), namespace, environmentID)
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	responses := make([]persistentVolumeClaimResponse, 0, len(claims))
	for index := range claims {
		responses = append(responses, h.pvcResponse(c.Request.Context(), &claims[index]))
	}
	model.Success(c, responses)
}

// ListPersistentVolumeClaimUsage reads local-path directory usage separately
// from the inventory so a slow or unreachable node never delays the storage UI.
func (h *StorageHandler) ListPersistentVolumeClaimUsage(c *gin.Context) {
	if h.pvc == nil {
		storageK8sUnavailable(c)
		return
	}
	claims, err := h.pvc.ListPVCsContext(c.Request.Context(), strings.TrimSpace(c.Query("namespace")))
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
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
		response := persistentVolumeClaimUsageResponse{
			Namespace:     claim.Namespace,
			Name:          claim.Name,
			CapacityBytes: pvcCapacityBytes(claim.Storage),
			Status:        "unavailable",
		}
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

	model.Success(c, responses)
}

func readLocalPersistentVolumeUsage(ctx context.Context, server *model.Server, localPath string, encKey []byte) (int64, error) {
	command := "sudo -n du -sb -- " + storageShellQuote(localPath)
	output, err := sshExec(ctx, 12*time.Second, append(buildSSHArgs(server, encKey, server.Host), command))
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

func (h *StorageHandler) CreatePersistentVolumeClaim(c *gin.Context) {
	if h.pvc == nil {
		storageK8sUnavailable(c)
		return
	}
	if h.store == nil {
		apiShared.InternalError(c, "数据存储未初始化")
		return
	}
	var request persistentVolumeClaimRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "名称和容量必填")
		return
	}
	namespace, err := h.Service.ResolveNamespace(request.Namespace, request.EnvironmentID)
	if err != nil {
		apiShared.BadRequest(c, err.Error())
		return
	}
	claim, err := h.Service.CreatePVCContext(c.Request.Context(), namespace, request.EnvironmentID, k8sclient.PersistentVolumeClaimRequest{Name: request.Name, Storage: request.Storage, StorageClassName: request.StorageClassName})
	if err != nil {
		apiShared.ValidationError(c, err.Error())
		return
	}
	info, err := h.Service.GetPVCContext(c.Request.Context(), namespace, claim.Name, request.EnvironmentID)
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, h.pvcResponse(c.Request.Context(), info))
}

func (h *StorageHandler) DeletePersistentVolumeClaim(c *gin.Context) {
	if h.pvc == nil {
		storageK8sUnavailable(c)
		return
	}
	if h.store == nil {
		apiShared.InternalError(c, "数据存储未初始化")
		return
	}
	var request persistentVolumeClaimRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		apiShared.BadRequest(c, "删除请求无效")
		return
	}
	namespace, err := h.Service.ResolveNamespace(request.Namespace, request.EnvironmentID)
	if err != nil {
		apiShared.BadRequest(c, err.Error())
		return
	}
	name := c.Param("name")
	claim, err := h.pvc.GetManagedPVCContext(c.Request.Context(), namespace, name, request.EnvironmentID)
	if err != nil {
		apiShared.NotFound(c, err.Error())
		return
	}
	migrationActive := false
	var activeMigration *model.PersistentVolumeMigration
	if request.EnvironmentID != 0 {
		if migration, migrationErr := h.Service.FindActiveMigration(request.EnvironmentID, name); migrationErr == nil {
			migrationActive, activeMigration = true, migration
		} else if !errors.Is(migrationErr, gorm.ErrRecordNotFound) {
			apiShared.DBError(c, "检查存储卷迁移状态失败")
			return
		}
	}
	references := h.pvcReferences(c.Request.Context(), request.EnvironmentID, namespace, name)
	if err := h.Service.ValidatePVCDeletion(request.ConfirmDataDelete, migrationActive, references, claim.ReclaimPolicy); err != nil {
		if activeMigration != nil {
			apiShared.ErrorWithData(c, http.StatusConflict, model.CodeConflict, err.Error(), activeMigration)
		} else if len(references) > 0 {
			apiShared.ErrorWithData(c, http.StatusConflict, model.CodeConflict, err.Error(), gin.H{"references": references})
		} else {
			apiShared.ErrorWithData(c, http.StatusConflict, model.CodeConflict, err.Error(), h.pvcResponse(c.Request.Context(), claim))
		}
		return
	}
	if err := h.Service.DeletePVCContext(c.Request.Context(), namespace, name, request.EnvironmentID); err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, gin.H{"name": name})
}

func (h *StorageHandler) ListStorageClasses(c *gin.Context) {
	if h.pvc == nil {
		storageK8sUnavailable(c)
		return
	}
	classes, err := h.pvc.ListStorageClassesContext(c.Request.Context())
	if err != nil {
		apiShared.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, classes)
}

func (h *StorageHandler) pvcEnvironment(c *gin.Context) (*model.Environment, bool) {
	if h.pvc == nil {
		storageK8sUnavailable(c)
		return nil, false
	}
	if h.store == nil {
		apiShared.InternalError(c, "数据存储未初始化")
		return nil, false
	}
	environmentID, err := apiShared.ParsePositiveID(strings.TrimSpace(c.Query("environment_id")))
	if err != nil {
		apiShared.BadRequest(c, "环境 ID 无效")
		return nil, false
	}
	environment, err := h.store.GetEnvironmentByID(environmentID)
	if err != nil {
		apiShared.NotFound(c, "环境不存在")
		return nil, false
	}
	return environment, true
}

func (h *StorageHandler) pvcRequestNamespace(ctx context.Context, request persistentVolumeClaimRequest) (string, error) {
	if request.EnvironmentID != 0 {
		environment, err := h.store.GetEnvironmentByID(request.EnvironmentID)
		if err != nil {
			return "", errors.New("环境不存在")
		}
		if namespace := strings.TrimSpace(request.Namespace); namespace != "" && namespace != environment.Namespace {
			return "", errors.New("命名空间与环境不匹配")
		}
		return environment.Namespace, nil
	}
	namespace := strings.TrimSpace(request.Namespace)
	if namespace == "" {
		return "", errors.New("请选择命名空间")
	}
	if h.workloads == nil || h.workloads.NamespaceExists(ctx, namespace) != nil {
		return "", errors.New("命名空间不存在或不可访问")
	}
	return namespace, nil
}

func (h *StorageHandler) pvcResponse(ctx context.Context, claim *k8sclient.PersistentVolumeClaimInfo) persistentVolumeClaimResponse {
	response := persistentVolumeClaimResponse{PersistentVolumeClaimInfo: *claim}
	if h.store != nil && claim.BoundNode != "" {
		if servers, err := h.store.ListServers(); err == nil {
			for _, server := range servers {
				if server.K8sNodeName == claim.BoundNode {
					response.BoundNodeDisplayName = server.Name
					break
				}
			}
		}
	}
	if h.store != nil && claim.EnvironmentID != 0 {
		if environment, err := h.store.GetEnvironmentByID(claim.EnvironmentID); err == nil {
			response.ProjectID = environment.ProjectID
			response.EnvironmentName = environment.Name
			if project, err := h.store.GetProject(environment.ProjectID); err == nil {
				response.ProjectName = project.Name
			}
		}
	}
	response.References = h.pvcReferences(ctx, claim.EnvironmentID, claim.Namespace, claim.Name)
	return response
}

func (h *StorageHandler) pvcReferences(ctx context.Context, environmentID uint, namespace, claimName string) []string {
	references := make([]string, 0)
	if h.store != nil {
		if applications, err := h.store.ListApplications(0, environmentID); err == nil {
			for _, app := range applications {
				if templates, err := h.store.ListApplicationDeploymentTemplates(app.ID); err == nil {
					for _, template := range templates {
						var spec application.ReleaseSpec
						if json.Unmarshal([]byte(template.Spec), &spec) == nil && volumeClaimReferenced(spec.Volumes, claimName) {
							references = append(references, fmt.Sprintf("模板：%s / %s", app.Name, template.Name))
						}
					}
				}
			}
		}
	}
	if h.workloads != nil {
		if deployments, err := h.workloads.ListDeployments(ctx, namespace); err == nil {
			for index := range deployments {
				if k8sclient.DeploymentReferencesPVC(&deployments[index], claimName) {
					references = append(references, "工作负载："+deployments[index].Name)
				}
			}
		}
		if statefulSets, err := h.workloads.ListStatefulSets(ctx, namespace); err == nil {
			for index := range statefulSets {
				if k8sclient.StatefulSetReferencesPVC(&statefulSets[index], claimName) {
					references = append(references, "工作负载："+statefulSets[index].Name)
				}
			}
		}
	}
	return references
}

func volumeClaimReferenced(volumes []application.VolumeMountSpec, claimName string) bool {
	for _, volume := range volumes {
		if volume.ClaimName == claimName {
			return true
		}
	}
	return false
}
