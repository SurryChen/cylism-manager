package logging

import (
	"fmt"

	"github.com/cylism/cylism-manager/internal/model"
)

type ScopeResolver interface {
	GetApplication(uint) (*model.Application, error)
	GetEnvironmentByID(uint) (*model.Environment, error)
	GetProject(uint) (*model.Project, error)
}

// ResolveApplicationScope validates related IDs and fills the canonical
// namespace/workload selectors used by LogQL.
func ResolveApplicationScope(req *QueryRequest, scope ScopeResolver) error {
	if req == nil || (req.ApplicationID == 0 && req.EnvironmentID == 0 && req.ProjectID == 0) {
		return nil
	}
	if scope == nil {
		return fmt.Errorf("日志应用范围校验不可用")
	}
	if req.ApplicationID != 0 {
		app, err := scope.GetApplication(req.ApplicationID)
		if err != nil {
			return fmt.Errorf("应用不存在")
		}
		if req.ProjectID != 0 && req.ProjectID != app.ProjectID {
			return fmt.Errorf("应用不属于所选项目")
		}
		if req.EnvironmentID != 0 && req.EnvironmentID != app.EnvironmentID {
			return fmt.Errorf("应用不属于所选环境")
		}
		if req.Namespace != "" && req.Namespace != app.Environment.Namespace {
			return fmt.Errorf("应用与命名空间不匹配")
		}
		if req.Workload != "" && req.Workload != app.Name {
			return fmt.Errorf("应用与工作负载不匹配")
		}
		req.ProjectID, req.EnvironmentID = app.ProjectID, app.EnvironmentID
		req.Namespace, req.Workload = app.Environment.Namespace, app.Name
	}
	if req.EnvironmentID != 0 {
		env, err := scope.GetEnvironmentByID(req.EnvironmentID)
		if err != nil {
			return fmt.Errorf("环境不存在")
		}
		if req.ProjectID != 0 && req.ProjectID != env.ProjectID {
			return fmt.Errorf("环境不属于所选项目")
		}
		if req.Namespace != "" && req.Namespace != env.Namespace {
			return fmt.Errorf("环境与命名空间不匹配")
		}
		req.ProjectID, req.Namespace = env.ProjectID, env.Namespace
	}
	if req.ProjectID != 0 {
		if _, err := scope.GetProject(req.ProjectID); err != nil {
			return fmt.Errorf("项目不存在")
		}
	}
	return nil
}
