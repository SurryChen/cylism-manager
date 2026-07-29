package application

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
)

type ResourceApplier interface {
	Preflight(ctx context.Context, application ApplicationContext, endpoint EndpointSpec) error
	Apply(ctx context.Context, resources *RenderedResources) error
	WaitReady(ctx context.Context, application ApplicationContext, spec ReleaseSpec) error
}

type Service struct {
	store   *store.Store
	applier ResourceApplier
}

func NewService(st *store.Store, applier ResourceApplier) *Service {
	return &Service{store: st, applier: applier}
}

func (s *Service) CreateRelease(ctx context.Context, applicationID, userID uint, spec ReleaseSpec) (*model.Release, error) {
	application, err := s.store.GetApplication(applicationID)
	if err != nil {
		return nil, fmt.Errorf("读取应用: %w", err)
	}
	if issues := ValidateReleaseSpec(spec); len(issues) > 0 {
		return nil, fmt.Errorf("发布定义无效: %s", issues[0].Message)
	}
	releases, err := s.store.ListReleases(applicationID)
	if err != nil {
		return nil, fmt.Errorf("读取发布历史: %w", err)
	}
	sequence := uint(1)
	if len(releases) > 0 {
		sequence = releases[0].Sequence + 1
	}
	resources, err := RenderResources(applicationContext(application, sequence), spec)
	if err != nil {
		return nil, err
	}
	snapshot, err := json.Marshal(resources.SanitizedSpec)
	if err != nil {
		return nil, fmt.Errorf("编码发布快照: %w", err)
	}
	var registryID *uint
	if spec.RegistryID != 0 {
		registryID = &spec.RegistryID
	}
	release := &model.Release{
		ApplicationID:   applicationID,
		Sequence:        sequence,
		Image:           spec.Image,
		ImageRegistryID: registryID,
		DesiredSpec:     string(snapshot),
		Status:          model.ReleaseStatusDraft,
		CreatedBy:       userID,
	}
	if err := s.store.CreateRelease(release); err != nil {
		return nil, fmt.Errorf("创建发布记录: %w", err)
	}
	return release, nil
}

func (s *Service) ExecuteRelease(ctx context.Context, releaseID uint, application *model.Application, spec ReleaseSpec) error {
	if s.applier == nil {
		return s.failRelease(releaseID, "preflight", fmt.Errorf("Kubernetes 发布器未初始化"))
	}
	release, err := s.store.GetRelease(releaseID)
	if err != nil {
		return err
	}
	applicationContext := applicationContext(application, release.Sequence)
	resources, err := RenderResources(applicationContext, spec)
	if err != nil {
		return s.failRelease(releaseID, "preflight", err)
	}
	if err := s.transition(release, model.ReleaseStatusValidating); err != nil {
		return err
	}
	if err := s.runStep(releaseID, "preflight", func() error { return s.applier.Preflight(ctx, applicationContext, spec.Endpoint) }); err != nil {
		return s.failRelease(releaseID, "preflight", err)
	}
	if err := s.transition(release, model.ReleaseStatusApplying); err != nil {
		return err
	}
	if err := s.runStep(releaseID, "apply_resources", func() error { return s.applier.Apply(ctx, resources) }); err != nil {
		return s.failRelease(releaseID, "apply_resources", err)
	}
	if err := s.transition(release, model.ReleaseStatusWaitingReady); err != nil {
		return err
	}
	if err := s.runStep(releaseID, "wait_ready", func() error { return s.applier.WaitReady(ctx, applicationContext, spec) }); err != nil {
		return s.failRelease(releaseID, "wait_ready", err)
	}
	return s.transition(release, model.ReleaseStatusSucceeded)
}

// RetryRelease 从既有 Release 的脱敏快照生成一个新的发布版本。
func (s *Service) RetryRelease(releaseID, userID uint) (*model.Release, ReleaseSpec, error) {
	release, err := s.store.GetRelease(releaseID)
	if err != nil {
		return nil, ReleaseSpec{}, err
	}
	spec, err := releaseSpecFromSnapshot(release.DesiredSpec)
	if err != nil {
		return nil, ReleaseSpec{}, err
	}
	retry, err := s.CreateRelease(context.Background(), release.ApplicationID, userID, spec)
	return retry, spec, err
}

// RollbackRelease 创建一个引用上一成功 Release 的新版本。
func (s *Service) RollbackRelease(releaseID, userID uint) (*model.Release, ReleaseSpec, error) {
	current, err := s.store.GetRelease(releaseID)
	if err != nil {
		return nil, ReleaseSpec{}, err
	}
	releases, err := s.store.ListReleases(current.ApplicationID)
	if err != nil {
		return nil, ReleaseSpec{}, err
	}
	var source *model.Release
	for _, candidate := range releases {
		if candidate.Sequence < current.Sequence && candidate.Status == model.ReleaseStatusSucceeded {
			source = &candidate
			break
		}
	}
	if source == nil {
		return nil, ReleaseSpec{}, fmt.Errorf("不存在可回滚的成功版本")
	}
	spec, err := releaseSpecFromSnapshot(source.DesiredSpec)
	if err != nil {
		return nil, ReleaseSpec{}, err
	}
	rollback, err := s.CreateRelease(context.Background(), current.ApplicationID, userID, spec)
	if err != nil {
		return nil, ReleaseSpec{}, err
	}
	rollback.SourceReleaseID = &source.ID
	if err := s.store.UpdateRelease(rollback); err != nil {
		return nil, ReleaseSpec{}, err
	}
	return rollback, spec, nil
}

func (s *Service) runStep(releaseID uint, step string, run func() error) error {
	now := time.Now()
	operation := &model.ReleaseOperation{ReleaseID: releaseID, Step: step, Status: model.ReleaseOperationRunning, StartedAt: &now}
	if err := s.store.CreateReleaseOperation(operation); err != nil {
		return err
	}
	operationLog := &model.OperationLog{ResourceType: "release", ResourceID: releaseID, Step: step, Status: model.ReleaseOperationRunning, CreatedAt: now}
	if err := s.store.CreateOperationLog(operationLog); err != nil {
		return err
	}
	err := run()
	completedAt := time.Now()
	operation.CompletedAt = &completedAt
	if err != nil {
		operation.Status = model.ReleaseOperationFailed
		operation.Detail = err.Error()
	} else {
		operation.Status = model.ReleaseOperationSuccess
	}
	operationLog.Status = operation.Status
	operationLog.Detail = operation.Detail
	if updateErr := s.store.UpdateReleaseOperation(operation); updateErr != nil {
		return updateErr
	}
	if updateErr := s.store.UpdateOperationLog(operationLog); updateErr != nil {
		return updateErr
	}
	return err
}

func (s *Service) transition(release *model.Release, status string) error {
	if !model.IsReleaseStatusValid(status) {
		return fmt.Errorf("无效发布状态: %s", status)
	}
	now := time.Now()
	if status == model.ReleaseStatusValidating && release.StartedAt == nil {
		release.StartedAt = &now
	}
	if status == model.ReleaseStatusSucceeded || status == model.ReleaseStatusFailed {
		release.CompletedAt = &now
	}
	release.Status = status
	return s.store.UpdateRelease(release)
}

func (s *Service) failRelease(releaseID uint, step string, cause error) error {
	release, err := s.store.GetRelease(releaseID)
	if err != nil {
		return cause
	}
	if release.Status != model.ReleaseStatusFailed {
		_ = s.transition(release, model.ReleaseStatusFailed)
	}
	return fmt.Errorf("%s: %w", step, cause)
}

func applicationContext(application *model.Application, sequence uint) ApplicationContext {
	return ApplicationContext{
		ProjectID: application.Project.ID, EnvironmentID: application.Environment.ID,
		ProjectName: application.Project.Name, EnvironmentName: application.Environment.Name,
		ApplicationName: application.Name, Namespace: application.Environment.Namespace, ReleaseSequence: sequence,
	}
}

func releaseSpecFromSnapshot(snapshot string) (ReleaseSpec, error) {
	var spec ReleaseSpec
	if err := json.Unmarshal([]byte(snapshot), &spec); err != nil {
		return ReleaseSpec{}, fmt.Errorf("读取发布快照: %w", err)
	}
	return spec, nil
}
