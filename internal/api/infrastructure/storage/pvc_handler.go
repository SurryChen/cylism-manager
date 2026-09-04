package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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

func (h *StorageHandler) ListPersistentVolumeClaims(c *gin.Context) {
	if h == nil || h.pvc == nil || h.Service == nil {
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
						var spec applicationservice.ReleaseSpec
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

func volumeClaimReferenced(volumes []applicationservice.VolumeMountSpec, claimName string) bool {
	for _, volume := range volumes {
		if volume.ClaimName == claimName {
			return true
		}
	}
	return false
}
