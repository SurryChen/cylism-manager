package store

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

// CreateProject persists a project and leaves registry ownership to the
// registry repository.
func (s *Store) CreateProject(project *model.Project) error {
	return s.db.Create(project).Error
}

func (s *Store) ListProjects() ([]model.Project, error) {
	var projects []model.Project
	err := s.db.Preload("Environments").Preload("DefaultImageRegistry").Order("created_at desc").Find(&projects).Error
	return projects, err
}

func (s *Store) GetProject(id uint) (*model.Project, error) {
	var project model.Project
	err := s.db.Preload("Environments").Preload("DefaultImageRegistry").First(&project, id).Error
	return &project, err
}

func (s *Store) UpdateProject(project *model.Project) error {
	return s.db.Save(project).Error
}

func (s *Store) DeleteProject(id uint) error {
	return s.db.Delete(&model.Project{}, id).Error
}

func (s *Store) CountProjectEnvironments(projectID uint) (int64, error) {
	var count int64
	err := s.db.Model(&model.Environment{}).Where("project_id = ?", projectID).Count(&count).Error
	return count, err
}

func (s *Store) CountProjectApplications(projectID uint) (int64, error) {
	var count int64
	err := s.db.Model(&model.Application{}).Where("project_id = ?", projectID).Count(&count).Error
	return count, err
}

func (s *Store) CreateEnvironment(environment *model.Environment) error {
	if environment == nil {
		return errors.New("environment is required")
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := ensureNamespaceAvailable(tx, environment.Namespace, 0); err != nil {
			return err
		}
		return tx.Create(environment).Error
	})
}

func (s *Store) ListEnvironments(projectID uint) ([]model.Environment, error) {
	var environments []model.Environment
	err := s.db.Where("project_id = ?", projectID).Order("created_at asc").Find(&environments).Error
	return environments, err
}

func (s *Store) GetEnvironment(projectID, environmentID uint) (*model.Environment, error) {
	var environment model.Environment
	err := s.db.Where("project_id = ?", projectID).First(&environment, environmentID).Error
	return &environment, err
}

func (s *Store) UpdateEnvironment(environment *model.Environment) error {
	if environment == nil {
		return errors.New("environment is required")
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := ensureNamespaceAvailable(tx, environment.Namespace, environment.ID); err != nil {
			return err
		}
		return tx.Save(environment).Error
	})
	if err != nil {
		return err
	}
	return s.ReconcileEnvironmentNamespaceUniqueness()
}

func (s *Store) DeleteEnvironment(id uint) error {
	if err := s.db.Delete(&model.Environment{}, id).Error; err != nil {
		return err
	}
	return s.ReconcileEnvironmentNamespaceUniqueness()
}

// EnsureNamespaceAvailable validates an intended namespace binding before Kubernetes is mutated.
func (s *Store) EnsureNamespaceAvailable(namespace string, excludeEnvironmentID uint) error {
	return ensureNamespaceAvailable(s.db, namespace, excludeEnvironmentID)
}

func ensureNamespaceAvailable(db *gorm.DB, namespace string, excludeEnvironmentID uint) error {
	var existing model.Environment
	query := db.Where("namespace = ?", namespace)
	if excludeEnvironmentID != 0 {
		query = query.Where("id <> ?", excludeEnvironmentID)
	}
	if err := query.Order("id asc").First(&existing).Error; err == nil {
		return &model.NamespaceConflictError{Namespace: namespace, EnvironmentID: existing.ID, ProjectID: existing.ProjectID}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return nil
}

// ListEnvironmentNamespaceConflicts returns duplicate legacy namespace bindings.
func (s *Store) ListEnvironmentNamespaceConflicts() ([]model.NamespaceConflict, error) {
	type duplicate struct{ Namespace string }
	var duplicates []duplicate
	if err := s.db.Model(&model.Environment{}).
		Select("namespace").
		Where("namespace <> ''").
		Group("namespace").
		Having("COUNT(*) > 1").
		Order("namespace asc").
		Find(&duplicates).Error; err != nil {
		return nil, err
	}
	conflicts := make([]model.NamespaceConflict, 0, len(duplicates))
	for _, duplicate := range duplicates {
		var environments []model.Environment
		if err := s.db.Where("namespace = ?", duplicate.Namespace).Order("project_id asc, id asc").Find(&environments).Error; err != nil {
			return nil, err
		}
		for index := range environments {
			environments[index].NamespaceConflict = true
		}
		conflicts = append(conflicts, model.NamespaceConflict{Namespace: duplicate.Namespace, Environments: environments})
	}
	return conflicts, nil
}

func (s *Store) IsEnvironmentNamespaceConflicted(namespace string) (bool, error) {
	var count int64
	if err := s.db.Model(&model.Environment{}).Where("namespace = ?", namespace).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 1, nil
}

// ReconcileEnvironmentNamespaceUniqueness creates the database backstop only after legacy duplicates are resolved.
func (s *Store) ReconcileEnvironmentNamespaceUniqueness() error {
	conflicts, err := s.ListEnvironmentNamespaceConflicts()
	if err != nil || len(conflicts) > 0 {
		return err
	}
	return s.db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_environments_namespace_unique ON environments(namespace)").Error
}

func (s *Store) CountEnvironmentApplications(environmentID uint) (int64, error) {
	var count int64
	err := s.db.Model(&model.Application{}).Where("environment_id = ?", environmentID).Count(&count).Error
	return count, err
}

func (s *Store) CreateApplication(application *model.Application) error {
	if err := application.SetCapabilities(application.Capabilities); err != nil {
		return err
	}
	return s.db.Create(application).Error
}

func (s *Store) GetApplication(id uint) (*model.Application, error) {
	var application model.Application
	err := s.db.Preload("Project.DefaultImageRegistry").Preload("Environment").Preload("Endpoints", func(db *gorm.DB) *gorm.DB { return db.Order("id asc") }).First(&application, id).Error
	if err == nil {
		application.LoadCapabilities()
	}
	return &application, err
}

func (s *Store) GetApplicationByEnvironmentName(environmentID uint, name string) (*model.Application, error) {
	var application model.Application
	err := s.db.Preload("Project.DefaultImageRegistry").Preload("Environment").Where("environment_id = ? AND name = ?", environmentID, name).First(&application).Error
	if err == nil {
		application.LoadCapabilities()
	}
	return &application, err
}

func (s *Store) ListApplications(scope ...uint) ([]model.Application, error) {
	var applications []model.Application
	query := s.db.Preload("Project.DefaultImageRegistry").Preload("Environment").Preload("Endpoints", func(db *gorm.DB) *gorm.DB { return db.Order("id asc") }).Order("created_at desc")
	if len(scope) > 0 && scope[0] != 0 {
		query = query.Where("project_id = ?", scope[0])
	}
	if len(scope) > 1 && scope[1] != 0 {
		query = query.Where("environment_id = ?", scope[1])
	}
	err := query.Find(&applications).Error
	if err == nil {
		for index := range applications {
			applications[index].LoadCapabilities()
		}
	}
	return applications, err
}

func (s *Store) CreateApplicationEndpoint(endpoint *model.ApplicationEndpoint) error {
	return s.db.Create(endpoint).Error
}

func (s *Store) ListApplicationEndpoints(applicationID uint) ([]model.ApplicationEndpoint, error) {
	var endpoints []model.ApplicationEndpoint
	err := s.db.Where("application_id = ?", applicationID).Order("id asc").Find(&endpoints).Error
	return endpoints, err
}

func (s *Store) GetApplicationEndpoint(applicationID, endpointID uint) (*model.ApplicationEndpoint, error) {
	var endpoint model.ApplicationEndpoint
	err := s.db.Where("application_id = ? AND id = ?", applicationID, endpointID).First(&endpoint).Error
	return &endpoint, err
}

func (s *Store) UpdateApplicationEndpoint(endpoint *model.ApplicationEndpoint) error {
	return s.db.Save(endpoint).Error
}

func (s *Store) DeleteApplicationEndpoint(applicationID, endpointID uint) error {
	return s.db.Where("application_id = ? AND id = ?", applicationID, endpointID).Delete(&model.ApplicationEndpoint{}).Error
}

func (s *Store) GetApplicationDeploymentTemplate(applicationID, templateID uint) (*model.ApplicationDeploymentTemplate, error) {
	var template model.ApplicationDeploymentTemplate
	err := s.db.Where("application_id = ? AND id = ?", applicationID, templateID).First(&template).Error
	return &template, err
}

func (s *Store) GetDefaultApplicationDeploymentTemplate(applicationID uint) (*model.ApplicationDeploymentTemplate, error) {
	var application model.Application
	if err := s.db.First(&application, applicationID).Error; err != nil {
		return nil, err
	}
	if application.DefaultDeploymentTemplateID == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return s.GetApplicationDeploymentTemplate(applicationID, *application.DefaultDeploymentTemplateID)
}

func (s *Store) ListApplicationDeploymentTemplates(applicationID uint) ([]model.ApplicationDeploymentTemplate, error) {
	var templates []model.ApplicationDeploymentTemplate
	err := s.db.Where("application_id = ?", applicationID).Order("created_at asc").Find(&templates).Error
	return templates, err
}

func (s *Store) CreateApplicationDeploymentTemplate(template *model.ApplicationDeploymentTemplate, makeDefault bool) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		template.Revision = 1
		if err := tx.Create(template).Error; err != nil {
			return err
		}
		if makeDefault {
			return tx.Model(&model.Application{}).Where("id = ?", template.ApplicationID).Update("default_deployment_template_id", template.ID).Error
		}
		return nil
	})
}

func (s *Store) UpdateApplicationDeploymentTemplate(template *model.ApplicationDeploymentTemplate) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var current model.ApplicationDeploymentTemplate
		if err := tx.Where("id = ? AND application_id = ?", template.ID, template.ApplicationID).First(&current).Error; err != nil {
			return err
		}
		template.CreatedAt = current.CreatedAt
		template.Revision = current.Revision + 1
		return tx.Save(template).Error
	})
}

// UpdateApplicationDeploymentTemplateIfRevision updates a template only when
// the caller still has the revision it read. Template content is the
// declarative source of truth for generated ConfigMaps.
func (s *Store) UpdateApplicationDeploymentTemplateIfRevision(template *model.ApplicationDeploymentTemplate, expectedRevision uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var current model.ApplicationDeploymentTemplate
		if err := tx.Where("id = ? AND application_id = ?", template.ID, template.ApplicationID).First(&current).Error; err != nil {
			return err
		}
		if current.Revision != expectedRevision {
			return &model.TemplateRevisionConflictError{Current: current.Revision, Expected: expectedRevision}
		}
		template.CreatedAt = current.CreatedAt
		template.Revision = current.Revision + 1
		return tx.Save(template).Error
	})
}

func (s *Store) SetDefaultApplicationDeploymentTemplate(applicationID, templateID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var template model.ApplicationDeploymentTemplate
		if err := tx.Where("id = ? AND application_id = ? AND enabled = ?", templateID, applicationID, true).First(&template).Error; err != nil {
			return err
		}
		return tx.Model(&model.Application{}).Where("id = ?", applicationID).Update("default_deployment_template_id", templateID).Error
	})
}

func (s *Store) DeleteApplicationDeploymentTemplate(applicationID, templateID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var releases int64
		if err := tx.Model(&model.Release{}).Where("template_id = ?", templateID).Count(&releases).Error; err != nil {
			return err
		}
		if releases > 0 {
			return fmt.Errorf("该模板已有关联发布记录，无法删除")
		}
		if err := tx.Where("id = ? AND application_id = ?", templateID, applicationID).Delete(&model.ApplicationDeploymentTemplate{}).Error; err != nil {
			return err
		}
		return tx.Model(&model.Application{}).Where("id = ? AND default_deployment_template_id = ?", applicationID, templateID).Update("default_deployment_template_id", nil).Error
	})
}

func (s *Store) CountApplicationEndpointsByDomain(domainID uint) (int64, error) {
	var count int64
	err := s.db.Model(&model.ApplicationEndpoint{}).Where("domain_id = ?", domainID).Count(&count).Error
	return count, err
}

func (s *Store) CountApplicationEndpointRoute(domainID uint, path string, exceptEndpointID uint) (int64, error) {
	var count int64
	// Metadata-only bindings describe an address for non-HTTP protocols and do
	// not occupy an HTTP Ingress route.
	query := s.db.Model(&model.ApplicationEndpoint{}).Where("domain_id = ? AND path = ? AND (ingress_mode IS NULL OR ingress_mode <> ?)", domainID, path, "metadata")
	if exceptEndpointID != 0 {
		query = query.Where("id <> ?", exceptEndpointID)
	}
	err := query.Count(&count).Error
	return count, err
}

type fileMountReferenceSpec struct {
	FileMounts []struct {
		SourceType string `json:"source_type"`
		SourceName string `json:"source_name"`
	} `json:"file_mounts"`
}

// ListResourceReferences finds application templates and releases that mount
// a named resource in the requested environment.
func (s *Store) ListResourceReferences(namespace, sourceType, sourceName string) ([]model.ResourceReference, error) {
	var applications []model.Application
	if err := s.db.Preload("Environment").Find(&applications).Error; err != nil {
		return nil, err
	}
	applicationByID := make(map[uint]model.Application)
	applicationIDs := make([]uint, 0, len(applications))
	for _, app := range applications {
		if app.Environment.Namespace == namespace {
			applicationByID[app.ID] = app
			applicationIDs = append(applicationIDs, app.ID)
		}
	}
	if len(applicationIDs) == 0 {
		return []model.ResourceReference{}, nil
	}
	var templates []model.ApplicationDeploymentTemplate
	if err := s.db.Where("application_id IN ?", applicationIDs).Find(&templates).Error; err != nil {
		return nil, err
	}
	var releases []model.Release
	if err := s.db.Where("application_id IN ?", applicationIDs).Find(&releases).Error; err != nil {
		return nil, err
	}
	matches := func(raw string) bool {
		var spec fileMountReferenceSpec
		if json.Unmarshal([]byte(raw), &spec) != nil {
			return false
		}
		for _, mount := range spec.FileMounts {
			if mount.SourceType == sourceType && mount.SourceName == sourceName {
				return true
			}
		}
		return false
	}
	result := make([]model.ResourceReference, 0)
	for _, template := range templates {
		if matches(template.Spec) {
			app := applicationByID[template.ApplicationID]
			result = append(result, model.ResourceReference{ApplicationID: app.ID, ApplicationName: app.Name, Kind: "template", Name: template.Name})
		}
	}
	for _, release := range releases {
		if matches(release.DesiredSpec) {
			app := applicationByID[release.ApplicationID]
			result = append(result, model.ResourceReference{ApplicationID: app.ID, ApplicationName: app.Name, Kind: "release", Name: fmt.Sprintf("Release #%d", release.Sequence)})
		}
	}
	return result, nil
}
