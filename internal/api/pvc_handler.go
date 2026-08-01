package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cylism/cylism-manager/internal/application"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type persistentVolumeClaimRequest struct {
	EnvironmentID     uint   `json:"environment_id"`
	Name              string `json:"name"`
	Storage           string `json:"storage"`
	StorageClassName  string `json:"storage_class_name"`
	ConfirmDataDelete bool   `json:"confirm_data_delete"`
}

type persistentVolumeClaimResponse struct {
	k8sclient.PersistentVolumeClaimInfo
	BoundNodeDisplayName string   `json:"bound_node_display_name,omitempty"`
	References           []string `json:"references,omitempty"`
}

func (h *K8sHandler) ListPersistentVolumeClaims(c *gin.Context) {
	environment, ok := h.pvcEnvironment(c)
	if !ok {
		return
	}
	claims, err := K8s.ListManagedPVCs(environment.Namespace, environment.ID)
	if err != nil {
		model.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	responses := make([]persistentVolumeClaimResponse, 0, len(claims))
	for index := range claims {
		responses = append(responses, h.pvcResponse(&claims[index], environment.ID))
	}
	model.Success(c, responses)
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
	if err := c.ShouldBindJSON(&request); err != nil || request.EnvironmentID == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "环境、名称和容量必填")
		return
	}
	environment, err := h.store.GetEnvironmentByID(request.EnvironmentID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "环境不存在")
		return
	}
	claim, err := K8s.CreateManagedPVC(environment.Namespace, environment.ID, k8sclient.PersistentVolumeClaimRequest{Name: request.Name, Storage: request.Storage, StorageClassName: request.StorageClassName})
	if err != nil {
		model.Error(c, http.StatusBadRequest, model.CodeValidationFail, err.Error())
		return
	}
	info, err := K8s.GetManagedPVC(environment.Namespace, claim.Name, environment.ID)
	if err != nil {
		model.Error(c, http.StatusOK, model.CodeK8sAPIError, err.Error())
		return
	}
	model.Success(c, h.pvcResponse(info, environment.ID))
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
	if err := c.ShouldBindJSON(&request); err != nil || request.EnvironmentID == 0 {
		model.Error(c, http.StatusBadRequest, model.CodeBadRequest, "环境不能为空")
		return
	}
	environment, err := h.store.GetEnvironmentByID(request.EnvironmentID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, "环境不存在")
		return
	}
	name := c.Param("name")
	claim, err := K8s.GetManagedPVC(environment.Namespace, name, environment.ID)
	if err != nil {
		model.Error(c, http.StatusNotFound, model.CodeNotFound, err.Error())
		return
	}
	references := h.pvcReferences(environment.ID, environment.Namespace, name)
	if len(references) > 0 {
		model.ErrorWithData(c, http.StatusConflict, model.CodeConflict, "PVC 仍被应用模板或工作负载引用，不能删除", gin.H{"references": references})
		return
	}
	if !request.ConfirmDataDelete {
		policy := claim.ReclaimPolicy
		if policy == "" {
			policy = "未知"
		}
		model.ErrorWithData(c, http.StatusConflict, model.CodeConflict, fmt.Sprintf("删除 PVC 可能影响底层数据（PV 回收策略：%s），请确认后重试", policy), h.pvcResponse(claim, environment.ID))
		return
	}
	if err := K8s.DeleteManagedPVC(environment.Namespace, name, environment.ID); err != nil {
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

func (h *K8sHandler) pvcResponse(claim *k8sclient.PersistentVolumeClaimInfo, environmentID uint) persistentVolumeClaimResponse {
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
	response.References = h.pvcReferences(environmentID, claim.Namespace, claim.Name)
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
