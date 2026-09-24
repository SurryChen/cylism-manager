package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	apiShared "github.com/cylism/cylism-manager/internal/api/shared"
	k8sclient "github.com/cylism/cylism-manager/internal/k8s"
	"github.com/cylism/cylism-manager/internal/model"
	applicationservice "github.com/cylism/cylism-manager/internal/service/application"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	appsv1 "k8s.io/api/apps/v1"
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

type persistentVolumeClaimReferencesResponse struct {
	Namespace  string   `json:"namespace"`
	Name       string   `json:"name"`
	References []string `json:"references"`
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
	projectID, err := apiShared.OptionalID(strings.TrimSpace(c.Query("project_id")))
	if err != nil {
		apiShared.BadRequest(c, "项目 ID 无效")
		return
	}
	claims, err := h.listPVCs(c.Request.Context(), namespace, environmentID, projectID)
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	_, hasPage := c.GetQuery("page")
	_, hasSize := c.GetQuery("size")
	if !hasPage && !hasSize {
		apiShared.Success(c, h.pvcResponses(c.Request.Context(), claims, false))
		return
	}
	page, size, offset := apiShared.Pagination(c)
	if offset > len(claims) {
		offset = len(claims)
	}
	end := offset + size
	if end > len(claims) {
		end = len(claims)
	}
	responses := h.pvcResponses(c.Request.Context(), claims[offset:end], false)
	apiShared.Success(c, gin.H{"items": responses, "total": len(claims), "page": page, "size": size})
}

// ListPersistentVolumeClaimReferences reads workload and template references
// independently of the inventory so slow Kubernetes reads do not delay it.
func (h *StorageHandler) ListPersistentVolumeClaimReferences(c *gin.Context) {
	if h == nil || h.pvc == nil {
		storageK8sUnavailable(c)
		return
	}
	claims, err := h.pvcUsageClaims(c.Request.Context(), c.QueryArray("claim"), strings.TrimSpace(c.Query("namespace")))
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	references := h.pvcReferenceIndex(c.Request.Context(), claims)
	responses := make([]persistentVolumeClaimReferencesResponse, 0, len(claims))
	for _, claim := range claims {
		claimReferences := references[pvcReferenceKey(claim.Namespace, claim.Name)]
		if claimReferences == nil {
			claimReferences = []string{}
		}
		responses = append(responses, persistentVolumeClaimReferencesResponse{
			Namespace:  claim.Namespace,
			Name:       claim.Name,
			References: claimReferences,
		})
	}
	apiShared.Success(c, responses)
}

// listPVCs keeps the Kubernetes read as narrow as the active scope permits.
// Project scope is resolved to its application environments, so it avoids a
// cluster-wide inventory followed by an in-memory project filter.
func (h *StorageHandler) listPVCs(ctx context.Context, namespace string, environmentID, projectID uint) ([]k8sclient.PersistentVolumeClaimInfo, error) {
	if projectID == 0 {
		return h.Service.ListPVCsContext(ctx, namespace, environmentID)
	}
	if h.store == nil {
		return nil, errors.New("数据存储未初始化")
	}
	project, err := h.store.GetProject(projectID)
	if err != nil {
		return nil, errors.New("项目不存在")
	}
	if environmentID != 0 {
		environment, environmentErr := h.store.GetEnvironmentByID(environmentID)
		if environmentErr != nil || environment.ProjectID != project.ID {
			return nil, errors.New("环境不属于当前项目")
		}
		return h.Service.ListPVCsContext(ctx, namespace, environmentID)
	}
	claims := make([]k8sclient.PersistentVolumeClaimInfo, 0)
	for _, environment := range project.Environments {
		items, listErr := h.Service.ListPVCsContext(ctx, environment.Namespace, environment.ID)
		if listErr != nil {
			return nil, listErr
		}
		for _, item := range items {
			if namespace == "" || item.Namespace == namespace {
				claims = append(claims, item)
			}
		}
	}
	sort.Slice(claims, func(i, j int) bool {
		if claims[i].Namespace != claims[j].Namespace {
			return claims[i].Namespace < claims[j].Namespace
		}
		return claims[i].Name < claims[j].Name
	})
	return claims, nil
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
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	apiShared.Success(c, h.pvcResponse(c.Request.Context(), info))
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
			apiShared.ErrorWithData(c, http.StatusConflict, apiShared.CodeConflict, err.Error(), apiShared.PersistentVolumeMigrationDTO(activeMigration))
		} else if len(references) > 0 {
			apiShared.ErrorWithData(c, http.StatusConflict, apiShared.CodeConflict, err.Error(), gin.H{"references": references})
		} else {
			apiShared.ErrorWithData(c, http.StatusConflict, apiShared.CodeConflict, err.Error(), h.pvcResponse(c.Request.Context(), claim))
		}
		return
	}
	if err := h.Service.DeletePVCContext(c.Request.Context(), namespace, name, request.EnvironmentID); err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	apiShared.Success(c, gin.H{"name": name})
}

func (h *StorageHandler) ListStorageClasses(c *gin.Context) {
	if h.pvc == nil {
		storageK8sUnavailable(c)
		return
	}
	classes, err := h.pvc.ListStorageClassesContext(c.Request.Context())
	if err != nil {
		apiShared.Error(c, http.StatusOK, apiShared.CodeK8sAPIError, err.Error())
		return
	}
	apiShared.Success(c, classes)
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
	responses := h.pvcResponses(ctx, []k8sclient.PersistentVolumeClaimInfo{*claim}, true)
	if len(responses) == 0 {
		return persistentVolumeClaimResponse{PersistentVolumeClaimInfo: *claim}
	}
	return responses[0]
}

func pvcReferenceKey(namespace, name string) string { return namespace + "\x00" + name }

// pvcResponses batches all list-only enrichment. The prior implementation
// repeated database and workload scans for every PVC in an inventory.
func (h *StorageHandler) pvcResponses(ctx context.Context, claims []k8sclient.PersistentVolumeClaimInfo, includeReferences bool) []persistentVolumeClaimResponse {
	responses := make([]persistentVolumeClaimResponse, len(claims))
	for index := range claims {
		responses[index] = persistentVolumeClaimResponse{PersistentVolumeClaimInfo: claims[index]}
	}
	if len(claims) == 0 {
		return responses
	}

	serversByNode := make(map[string]string)
	environments := make(map[uint]*model.Environment)
	projects := make(map[uint]string)
	if h.store != nil {
		if servers, err := h.store.ListServers(); err == nil {
			for _, server := range servers {
				serversByNode[server.K8sNodeName] = server.Name
			}
		}
		for _, claim := range claims {
			if claim.EnvironmentID == 0 || environments[claim.EnvironmentID] != nil {
				continue
			}
			if environment, err := h.store.GetEnvironmentByID(claim.EnvironmentID); err == nil {
				environments[claim.EnvironmentID] = environment
				if _, found := projects[environment.ProjectID]; !found {
					if project, projectErr := h.store.GetProject(environment.ProjectID); projectErr == nil {
						projects[environment.ProjectID] = project.Name
					}
				}
			}
		}
	}

	var references map[string][]string
	if includeReferences {
		references = h.pvcReferenceIndex(ctx, claims)
	}
	for index := range responses {
		claim := &responses[index].PersistentVolumeClaimInfo
		responses[index].BoundNodeDisplayName = serversByNode[claim.BoundNode]
		if environment := environments[claim.EnvironmentID]; environment != nil {
			responses[index].ProjectID = environment.ProjectID
			responses[index].EnvironmentName = environment.Name
			responses[index].ProjectName = projects[environment.ProjectID]
		}
		if includeReferences {
			responses[index].References = references[pvcReferenceKey(claim.Namespace, claim.Name)]
		}
	}
	return responses
}

func (h *StorageHandler) pvcReferenceIndex(ctx context.Context, claims []k8sclient.PersistentVolumeClaimInfo) map[string][]string {
	references := make(map[string][]string)
	add := func(namespace, name, reference string) {
		key := pvcReferenceKey(namespace, name)
		references[key] = append(references[key], reference)
	}

	if h.store != nil {
		environmentClaims := make(map[uint][]k8sclient.PersistentVolumeClaimInfo)
		for _, claim := range claims {
			if claim.EnvironmentID != 0 {
				environmentClaims[claim.EnvironmentID] = append(environmentClaims[claim.EnvironmentID], claim)
			}
		}
		for environmentID, scopedClaims := range environmentClaims {
			applications, err := h.store.ListApplications(0, environmentID)
			if err != nil {
				continue
			}
			for _, application := range applications {
				templates, templateErr := h.store.ListApplicationDeploymentTemplates(application.ID)
				if templateErr != nil {
					continue
				}
				for _, template := range templates {
					var spec applicationservice.ReleaseSpec
					if json.Unmarshal([]byte(template.Spec), &spec) != nil {
						continue
					}
					for _, claim := range scopedClaims {
						if volumeClaimReferenced(spec.Volumes, claim.Name) {
							add(claim.Namespace, claim.Name, fmt.Sprintf("模板：%s / %s", application.Name, template.Name))
						}
					}
				}
			}
		}
	}

	if h.workloads != nil {
		claimsByNamespace := make(map[string][]k8sclient.PersistentVolumeClaimInfo)
		for _, claim := range claims {
			claimsByNamespace[claim.Namespace] = append(claimsByNamespace[claim.Namespace], claim)
		}
		for key, values := range h.pvcWorkloadReferenceIndex(ctx, claimsByNamespace) {
			for _, value := range values {
				references[key] = append(references[key], value)
			}
		}
	}
	return references
}

const maxConcurrentPVCReferenceRequests = 4

func (h *StorageHandler) pvcWorkloadReferenceIndex(ctx context.Context, claimsByNamespace map[string][]k8sclient.PersistentVolumeClaimInfo) map[string][]string {
	references := make(map[string][]string)
	if len(claimsByNamespace) == 0 {
		return references
	}
	type namespaceReferences map[string][]string
	results := make(chan namespaceReferences, len(claimsByNamespace))
	requests := make(chan struct{}, maxConcurrentPVCReferenceRequests)
	var waitGroup sync.WaitGroup
	for namespace, claims := range claimsByNamespace {
		waitGroup.Add(1)
		go func(namespace string, claims []k8sclient.PersistentVolumeClaimInfo) {
			defer waitGroup.Done()
			var deployments []appsv1.Deployment
			var statefulSets []appsv1.StatefulSet
			var resourceWaitGroup sync.WaitGroup
			resourceWaitGroup.Add(2)
			go func() {
				defer resourceWaitGroup.Done()
				requests <- struct{}{}
				defer func() { <-requests }()
				deployments, _ = h.workloads.ListDeployments(ctx, namespace)
			}()
			go func() {
				defer resourceWaitGroup.Done()
				requests <- struct{}{}
				defer func() { <-requests }()
				statefulSets, _ = h.workloads.ListStatefulSets(ctx, namespace)
			}()
			resourceWaitGroup.Wait()

			result := make(namespaceReferences)
			add := func(name, reference string) {
				key := pvcReferenceKey(namespace, name)
				result[key] = append(result[key], reference)
			}
			for index := range deployments {
				for _, claim := range claims {
					if k8sclient.DeploymentReferencesPVC(&deployments[index], claim.Name) {
						add(claim.Name, "工作负载："+deployments[index].Name)
					}
				}
			}
			for index := range statefulSets {
				for _, claim := range claims {
					if k8sclient.StatefulSetReferencesPVC(&statefulSets[index], claim.Name) {
						add(claim.Name, "工作负载："+statefulSets[index].Name)
					}
				}
			}
			results <- result
		}(namespace, claims)
	}
	waitGroup.Wait()
	close(results)
	for result := range results {
		for key, values := range result {
			references[key] = append(references[key], values...)
		}
	}
	return references
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
