package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cylism/cylism-manager/internal/application"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

var readPersistentVolumeUsage = readLocalPersistentVolumeUsage

func (h *K8sHandler) ListPersistentVolumeClaims(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	environmentID, err := optionalQueryID(c, "environment_id")
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "环境 ID 无效")
		return
	}
	namespace := strings.TrimSpace(c.Query("namespace"))
	var claims []k8sclient.PersistentVolumeClaimInfo
	if environmentID != 0 {
		if h.store == nil {
			model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "数据存储未初始化")
			return
		}
		environment, err := h.store.GetEnvironmentByID(environmentID)
		if err != nil {
			model.Error(c, http.StatusNotFound, model.CodeNotFound, "环境不存在")
			return
		}
		if namespace != "" && namespace != environment.Namespace {
			model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "命名空间与环境不匹配")
			return
		}
		claims, err = K8s.ListManagedPVCs(environment.Namespace, environment.ID)
	} else {
		claims, err = K8s.ListPVCs(namespace)
	}
	if err != nil {
		model.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	responses := make([]persistentVolumeClaimResponse, 0, len(claims))
	for index := range claims {
		responses = append(responses, h.pvcResponse(&claims[index]))
	}
	model.Success(c, responses)
}

// ListPersistentVolumeClaimUsage reads local-path directory usage separately
// from the inventory so a slow or unreachable node never delays the storage UI.
func (h *K8sHandler) ListPersistentVolumeClaimUsage(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	claims, err := K8s.ListPVCs(strings.TrimSpace(c.Query("namespace")))
	if err != nil {
		model.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
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
				usedBytes, readErr := readPersistentVolumeUsage(&job.server, job.claim.LocalPath, h.encKey)
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

func readLocalPersistentVolumeUsage(server *model.Server, localPath string, encKey []byte) (int64, error) {
	command := "sudo -n du -sb -- " + shellQuote(localPath)
	output, err := sshExec(12*time.Second, append(buildSSHArgs(server, encKey, server.Host), command))
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

func (h *K8sHandler) CreatePersistentVolumeClaim(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	if h.store == nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "数据存储未初始化")
		return
	}
	var request persistentVolumeClaimRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "名称和容量必填")
		return
	}
	namespace, err := h.pvcRequestNamespace(request)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, err.Error())
		return
	}
	claim, err := K8s.CreateManagedPVC(namespace, request.EnvironmentID, k8sclient.PersistentVolumeClaimRequest{Name: request.Name, Storage: request.Storage, StorageClassName: request.StorageClassName})
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	info, err := K8s.GetManagedPVC(namespace, claim.Name, request.EnvironmentID)
	if err != nil {
		model.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, h.pvcResponse(info))
}

func (h *K8sHandler) DeletePersistentVolumeClaim(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	if h.store == nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "数据存储未初始化")
		return
	}
	var request persistentVolumeClaimRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "删除请求无效")
		return
	}
	namespace, err := h.pvcRequestNamespace(request)
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, err.Error())
		return
	}
	name := c.Param("name")
	claim, err := K8s.GetManagedPVC(namespace, name, request.EnvironmentID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, err.Error())
		return
	}
	if request.EnvironmentID != 0 {
		if migration, migrationErr := h.store.FindActivePVCMigration(request.EnvironmentID, name); migrationErr == nil {
			model.ErrorWithData(c, http.StatusConflict, model.CodeConflict, "存储卷正在迁移，不能删除", migration)
			return
		} else if !errors.Is(migrationErr, gorm.ErrRecordNotFound) {
			model.Error(c, http.StatusInternalServerError, model.CodeDBError, "检查存储卷迁移状态失败")
			return
		}
	}
	references := h.pvcReferences(request.EnvironmentID, namespace, name)
	if len(references) > 0 {
		model.ErrorWithData(c, http.StatusConflict, model.CodeConflict, "PVC 仍被应用模板或工作负载引用，不能删除", gin.H{"references": references})
		return
	}
	if !request.ConfirmDataDelete {
		policy := claim.ReclaimPolicy
		if policy == "" {
			policy = "未知"
		}
		model.ErrorWithData(c, http.StatusConflict, model.CodeConflict, fmt.Sprintf("删除 PVC 可能影响底层数据（PV 回收策略：%s），请确认后重试", policy), h.pvcResponse(claim))
		return
	}
	if err := K8s.DeleteManagedPVC(namespace, name, request.EnvironmentID); err != nil {
		model.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, gin.H{"name": name})
}

func (h *K8sHandler) ListStorageClasses(c *gin.Context) {
	if K8s == nil {
		k8sUnavailable(c)
		return
	}
	classes, err := K8s.ListStorageClasses()
	if err != nil {
		model.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, classes)
}

func (h *K8sHandler) pvcEnvironment(c *gin.Context) (*model.Environment, bool) {
	if K8s == nil {
		k8sUnavailable(c)
		return nil, false
	}
	if h.store == nil {
		model.Error(c, http.StatusInternalServerError, model.CodeInternalError, "数据存储未初始化")
		return nil, false
	}
	environmentID, err := parseID(c.Query("environment_id"))
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "环境 ID 无效")
		return nil, false
	}
	environment, err := h.store.GetEnvironmentByID(environmentID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "环境不存在")
		return nil, false
	}
	return environment, true
}

func (h *K8sHandler) pvcRequestNamespace(request persistentVolumeClaimRequest) (string, error) {
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
	if _, err := K8s.Clientset.CoreV1().Namespaces().Get(K8s.Ctx(), namespace, metav1.GetOptions{}); err != nil {
		return "", errors.New("命名空间不存在或不可访问")
	}
	return namespace, nil
}

func (h *K8sHandler) pvcResponse(claim *k8sclient.PersistentVolumeClaimInfo) persistentVolumeClaimResponse {
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
	response.References = h.pvcReferences(claim.EnvironmentID, claim.Namespace, claim.Name)
	return response
}

func (h *K8sHandler) pvcReferences(environmentID uint, namespace, claimName string) []string {
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
	if K8s != nil {
		if deployments, err := K8s.Clientset.AppsV1().Deployments(namespace).List(K8s.Ctx(), metav1.ListOptions{}); err == nil {
			for index := range deployments.Items {
				if deploymentClaimReferenced(&deployments.Items[index], claimName) {
					references = append(references, "工作负载："+deployments.Items[index].Name)
				}
			}
		}
		if statefulSets, err := K8s.Clientset.AppsV1().StatefulSets(namespace).List(K8s.Ctx(), metav1.ListOptions{}); err == nil {
			for index := range statefulSets.Items {
				if statefulSetClaimReferenced(&statefulSets.Items[index], claimName) {
					references = append(references, "工作负载："+statefulSets.Items[index].Name)
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

func deploymentClaimReferenced(deployment *appsv1.Deployment, claimName string) bool {
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == claimName {
			return true
		}
	}
	return false
}

func statefulSetClaimReferenced(statefulSet *appsv1.StatefulSet, claimName string) bool {
	for _, volume := range statefulSet.Spec.Template.Spec.Volumes {
		if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == claimName {
			return true
		}
	}
	return false
}
