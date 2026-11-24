package store

import (
	"fmt"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

func (s *Store) ListApplicationCatalogItems() ([]model.ApplicationCatalogItem, error) {
	var items []model.ApplicationCatalogItem
	err := s.db.Order("name asc, id asc").Find(&items).Error
	return items, err
}

func (s *Store) GetApplicationCatalogItem(id uint) (*model.ApplicationCatalogItem, error) {
	var item model.ApplicationCatalogItem
	err := s.db.First(&item, id).Error
	return &item, err
}

func (s *Store) CreateApplicationCatalogItem(item *model.ApplicationCatalogItem) error {
	item.Revision = 1
	return s.db.Create(item).Error
}

func (s *Store) UpdateApplicationCatalogItem(item *model.ApplicationCatalogItem, expectedRevision uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.ApplicationCatalogItem{}).
			Where("id = ? AND revision = ?", item.ID, expectedRevision).
			Updates(map[string]interface{}{
				"name": item.Name, "application_name": item.Slug,
				"description": item.Description, "components": item.ComponentsData,
				"entry_component": item.EntryComponent, "spec": item.LegacySpec,
				"enabled":  item.Enabled,
				"revision": expectedRevision + 1,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("商城条目已被修改，请刷新后重试")
		}
		item.Revision = expectedRevision + 1
		return nil
	})
}

func (s *Store) DeleteApplicationCatalogItem(id uint) error {
	return s.db.Delete(&model.ApplicationCatalogItem{}, id).Error
}

func (s *Store) ListApplicationCatalogInstallations(projectID, environmentID uint) ([]model.ApplicationCatalogInstallation, error) {
	var installations []model.ApplicationCatalogInstallation
	if err := s.db.Where("project_id = ? AND environment_id = ?", projectID, environmentID).Order("id desc").Find(&installations).Error; err != nil {
		return nil, err
	}
	for index := range installations {
		if err := s.db.Where("installation_id = ?", installations[index].ID).Order("id asc").Find(&installations[index].Components).Error; err != nil {
			return nil, err
		}
	}
	return installations, nil
}

// ImportApplicationCatalogItems creates packages, components and default templates atomically.
func (s *Store) ImportApplicationCatalogItems(projectID, environmentID, userID uint, items []model.ApplicationCatalogItem) ([]model.ApplicationCatalogInstallation, error) {
	installations := make([]model.ApplicationCatalogInstallation, 0, len(items))
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing []string
		if err := tx.Model(&model.Application{}).Where("environment_id = ?", environmentID).Pluck("name", &existing).Error; err != nil {
			return err
		}
		used := make(map[string]bool, len(existing))
		for _, name := range existing {
			used[name] = true
		}
		for _, item := range items {
			if !item.Enabled {
				return fmt.Errorf("商城应用 %s 尚未启用", item.Name)
			}
			components, err := item.CatalogComponents()
			if err != nil {
				return fmt.Errorf("读取 %s 的组件失败: %w", item.Name, err)
			}
			if len(components) == 0 {
				return fmt.Errorf("商城应用 %s 尚未配置部署组件", item.Name)
			}
			installation := model.ApplicationCatalogInstallation{CatalogItemID: item.ID, CatalogRevision: item.Revision, Name: item.Name, ProjectID: projectID, EnvironmentID: environmentID}
			if err := tx.Create(&installation).Error; err != nil {
				return err
			}
			for _, component := range components {
				if used[component.ApplicationName] {
					return fmt.Errorf("部署组件 %s 已存在于当前环境", component.ApplicationName)
				}
				used[component.ApplicationName] = true
				app := model.Application{ProjectID: projectID, EnvironmentID: environmentID, Name: component.ApplicationName, WorkloadKind: "deployment", CreatedBy: userID}
				if err := app.SetCapabilities(nil); err != nil {
					return err
				}
				if err := tx.Create(&app).Error; err != nil {
					return err
				}
				template := model.ApplicationDeploymentTemplate{ApplicationID: app.ID, Name: "默认配置", Description: fmt.Sprintf("来自商城应用：%s（v%d）", item.Name, item.Revision), Enabled: true, DefaultVersion: component.DefaultVersion, Spec: string(component.Spec), Revision: 1, UpdatedBy: userID}
				if err := tx.Create(&template).Error; err != nil {
					return err
				}
				if err := tx.Model(&app).Update("default_deployment_template_id", template.ID).Error; err != nil {
					return err
				}
				entry := component.ApplicationName == item.CatalogEntryComponent()
				if entry {
					installation.EntryApplicationID = app.ID
				}
				link := model.ApplicationCatalogInstallationComponent{InstallationID: installation.ID, ApplicationID: app.ID, Name: component.Name, IsEntry: entry}
				if err := tx.Create(&link).Error; err != nil {
					return err
				}
				installation.Components = append(installation.Components, link)
			}
			if installation.EntryApplicationID == 0 {
				return fmt.Errorf("商城应用 %s 缺少有效入口组件", item.Name)
			}
			if err := tx.Model(&installation).Update("entry_application_id", installation.EntryApplicationID).Error; err != nil {
				return err
			}
			installations = append(installations, installation)
		}
		return nil
	})
	return installations, err
}
