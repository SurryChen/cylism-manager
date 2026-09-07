package store

import (
	"errors"
	"fmt"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/runtime"
	"gorm.io/gorm"
)

// backfillData applies idempotent data upgrades after the schema is ready.
func (s *Store) backfillData() error {
	if err := s.backfillManagedDomainEnvironments(); err != nil {
		return err
	}
	if err := s.backfillApplicationDeploymentTemplates(); err != nil {
		return err
	}
	if err := s.backfillRuntimeVersions(); err != nil {
		return err
	}
	return s.ReconcileEnvironmentNamespaceUniqueness()
}

func (s *Store) backfillRuntimeVersions() error {
	var runtimes []model.RuntimeInstance
	if err := s.db.Where("runtime_version = ?", "").Find(&runtimes).Error; err != nil {
		return fmt.Errorf("查询待回填的 Runtime 版本: %w", err)
	}
	for index := range runtimes {
		version := runtime.ImageVersion(runtimes[index].Image)
		if version == "" {
			continue
		}
		if err := s.db.Model(&runtimes[index]).Update("runtime_version", version).Error; err != nil {
			return fmt.Errorf("回填 Runtime %s 版本: %w", runtimes[index].Name, err)
		}
	}
	return nil
}

func (s *Store) backfillApplicationDeploymentTemplates() error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var templates []model.ApplicationDeploymentTemplate
		if err := tx.Where("name = ''").Find(&templates).Error; err != nil {
			return err
		}
		for index := range templates {
			template := &templates[index]
			template.Name = "默认模板"
			template.Enabled = true
			if err := tx.Save(template).Error; err != nil {
				return err
			}
		}
		var applications []model.Application
		if err := tx.Where("default_deployment_template_id IS NULL").Find(&applications).Error; err != nil {
			return err
		}
		for index := range applications {
			var template model.ApplicationDeploymentTemplate
			err := tx.Where("application_id = ?", applications[index].ID).Order("id asc").First(&template).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if err := tx.Model(&applications[index]).Update("default_deployment_template_id", template.ID).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) backfillManagedDomainEnvironments() error {
	var domains []model.ManagedDomain
	if err := s.db.Where("environment_id = 0 AND namespace <> ''").Find(&domains).Error; err != nil {
		return err
	}
	for index := range domains {
		var environments []model.Environment
		if err := s.db.Where("namespace = ?", domains[index].Namespace).Find(&environments).Error; err != nil {
			return err
		}
		if len(environments) == 1 {
			if err := s.db.Model(&domains[index]).Update("environment_id", environments[0].ID).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
