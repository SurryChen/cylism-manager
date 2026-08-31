package store

import (
	"encoding/json"
	"fmt"

	"github.com/cylism/cylism-manager/internal/model"
)

type fileMountReferenceSpec struct {
	FileMounts []struct {
		SourceType string `json:"source_type"`
		SourceName string `json:"source_name"`
	} `json:"file_mounts"`
}

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
