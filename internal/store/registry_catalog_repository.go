package store

import (
	"encoding/json"
	"fmt"

	"github.com/cylism/cylism-manager/internal/model"
)

// ListManagedRegistryContentReferences loads persisted deployment inputs used
// to protect a managed Registry manifest from deletion.
func (s *Store) ListManagedRegistryContentReferences() ([]model.ManagedRegistryContentReference, error) {
	var releases []model.Release
	if err := s.db.Find(&releases).Error; err != nil {
		return nil, err
	}
	result := make([]model.ManagedRegistryContentReference, 0, len(releases))
	for _, release := range releases {
		result = append(result, model.ManagedRegistryContentReference{Kind: "release", Name: fmt.Sprintf("发布 #%d", release.Sequence), Image: release.Image, Digest: release.ImageDigest})
	}
	var templates []model.ApplicationDeploymentTemplate
	if err := s.db.Find(&templates).Error; err != nil {
		return nil, err
	}
	for _, template := range templates {
		var spec struct {
			Image string `json:"image"`
		}
		if err := json.Unmarshal([]byte(template.Spec), &spec); err != nil {
			result = append(result, model.ManagedRegistryContentReference{Kind: "template", Name: template.Name})
			continue
		}
		if spec.Image != "" {
			result = append(result, model.ManagedRegistryContentReference{Kind: "template", Name: template.Name, Image: spec.Image})
		}
	}
	return result, nil
}
