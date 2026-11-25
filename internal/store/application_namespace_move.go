package store

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

// CheckApplicationNamespaceMove keeps project ownership and physical scope
// independent until adoption has succeeded.
func (s *Store) CheckApplicationNamespaceMove(move model.ApplicationNamespaceMove) error {
	_, _, err := s.checkApplicationNamespaceMove(move)
	return err
}

func (s *Store) checkApplicationNamespaceMove(move model.ApplicationNamespaceMove) (*model.Application, *model.Environment, error) {
	app, err := s.GetApplication(move.ApplicationID)
	if err != nil {
		return nil, nil, err
	}
	if move.OwnerID == 0 || app.Project.OwnerID != move.OwnerID {
		return nil, nil, gorm.ErrRecordNotFound
	}
	target, err := s.GetEnvironment(app.OwningProjectID(), move.TargetEnvironmentID)
	if err != nil {
		return nil, nil, err
	}
	project, err := s.GetProject(target.ProjectID)
	if err != nil {
		return nil, nil, err
	}
	if project.OwnerID != move.OwnerID || target.ClusterID == 0 || target.ClusterID != app.Environment.ClusterID {
		return nil, nil, model.ErrApplicationTransferTarget
	}
	if app.EnvironmentID == target.ID {
		return app, target, nil
	}
	if app.EnvironmentID != move.SourceEnvironmentID || app.Environment.Namespace == target.Namespace {
		return nil, nil, model.ErrApplicationNamespaceMoveChanged
	}
	var latest model.Release
	if err := s.db.Where("application_id = ? AND status = ?", app.ID, model.ReleaseStatusSucceeded).Order("sequence DESC").First(&latest).Error; err != nil {
		return nil, nil, err
	}
	if latest.ID != move.ReleaseID {
		return nil, nil, model.ErrApplicationNamespaceMoveChanged
	}
	var active int64
	if err := s.db.Model(&model.Release{}).Where("application_id = ? AND status NOT IN ?", app.ID, []string{
		model.ReleaseStatusSucceeded, model.ReleaseStatusFailed, model.ReleaseStatusRolledBack,
	}).Count(&active).Error; err != nil {
		return nil, nil, err
	}
	if active > 0 {
		return nil, nil, model.ErrApplicationTransferBusy
	}
	var existing int64
	if err := s.db.Model(&model.Application{}).Where("environment_id = ? AND name = ?", target.ID, app.Name).Count(&existing).Error; err != nil {
		return nil, nil, err
	}
	if existing > 0 {
		return nil, nil, model.ErrApplicationNamespaceMoveChanged
	}
	return app, target, nil
}

func (s *Store) AdoptApplicationNamespace(move model.ApplicationNamespaceMove) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		app, target, err := (&Store{db: tx}).checkApplicationNamespaceMove(move)
		if err != nil || app.EnvironmentID == target.ID {
			return err
		}
		domains, err := (&Store{db: tx}).planApplicationNamespaceMoveDomains(app, target)
		if err != nil {
			return err
		}
		if !slices.Equal(domains, move.DomainIDs) {
			return model.ErrApplicationNamespaceMoveChanged
		}
		if move.MaintenanceID == 0 {
			if err := ensureNoNamespaceMaintenance(tx, app.ID); err != nil {
				return err
			}
		} else {
			result := tx.Where("id = ? AND application_id = ? AND target_environment_id = ? AND phase = ?", move.MaintenanceID, app.ID, target.ID, model.NamespaceMoveRouted).Delete(&model.ApplicationNamespaceMaintenance{})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return model.ErrApplicationNamespaceMoveChanged
			}
		}
		result := tx.Model(&model.Application{}).Where("id = ? AND environment_id = ? AND "+applicationOwningProject+" = ?", app.ID, move.SourceEnvironmentID, target.ProjectID).
			Updates(map[string]interface{}{"project_id": target.ProjectID, "environment_id": target.ID, "management_project_id": nil})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return model.ErrApplicationNamespaceMoveChanged
		}
		for _, id := range move.DomainIDs {
			result := tx.Model(&model.ManagedDomain{}).Where("id = ? AND environment_id = ?", id, app.EnvironmentID).
				Updates(map[string]interface{}{"environment_id": target.ID, "namespace": target.Namespace})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return model.ErrApplicationNamespaceMoveChanged
			}
		}
		return nil
	})
}

// PlanApplicationNamespaceMoveDomains moves a shared domain with its sole HTTP
// ingress. Metadata-only bindings keep their ID and can follow independently.
func (s *Store) PlanApplicationNamespaceMoveDomains(move model.ApplicationNamespaceMove) ([]uint, error) {
	app, target, err := s.checkApplicationNamespaceMove(move)
	if err != nil {
		return nil, err
	}
	return s.planApplicationNamespaceMoveDomains(app, target)
}

func (s *Store) planApplicationNamespaceMoveDomains(app *model.Application, target *model.Environment) ([]uint, error) {
	var domains []uint
	for _, endpoint := range app.Endpoints {
		if endpoint.DomainID != 0 && !slices.Contains(domains, endpoint.DomainID) {
			domains = append(domains, endpoint.DomainID)
		}
	}
	slices.Sort(domains)
	var transfer []uint
	for _, id := range domains {
		domain, err := s.GetManagedDomain(id)
		if err != nil {
			return nil, err
		}
		var bindings []struct {
			ApplicationID uint
			EnvironmentID uint
			ProjectID     uint
			Mode          string
		}
		if err := s.db.Table("application_endpoints").
			Select("application_endpoints.application_id, applications.environment_id, "+applicationOwningProject+" AS project_id, application_endpoints.ingress_mode AS mode").
			Joins("JOIN applications ON applications.id = application_endpoints.application_id").
			Where("application_endpoints.domain_id = ?", id).Scan(&bindings).Error; err != nil {
			return nil, err
		}
		var ingressOwner uint
		otherAtSource := false
		for _, binding := range bindings {
			if binding.ProjectID != target.ProjectID || (binding.EnvironmentID != app.EnvironmentID && binding.EnvironmentID != target.ID) {
				return nil, fmt.Errorf("域名 %s 还关联其他项目或运行环境的应用", domain.Hostname)
			}
			if binding.ApplicationID != app.ID && binding.EnvironmentID == app.EnvironmentID {
				otherAtSource = true
			}
			if strings.EqualFold(strings.TrimSpace(binding.Mode), "metadata") {
				continue
			}
			if ingressOwner != 0 && ingressOwner != binding.ApplicationID {
				return nil, fmt.Errorf("域名 %s 有多个 Ingress 应用，需要一同切换路由", domain.Hostname)
			}
			ingressOwner = binding.ApplicationID
			if binding.EnvironmentID != domain.EnvironmentID {
				return nil, fmt.Errorf("域名 %s 的归属与 Ingress 环境不一致", domain.Hostname)
			}
		}
		switch domain.EnvironmentID {
		case target.ID:
			continue
		case app.EnvironmentID:
			if ingressOwner == app.ID || (ingressOwner == 0 && !otherAtSource) {
				transfer = append(transfer, id)
			}
		default:
			return nil, fmt.Errorf("域名 %s 的归属与迁移环境不一致", domain.Hostname)
		}
	}
	return transfer, nil
}
