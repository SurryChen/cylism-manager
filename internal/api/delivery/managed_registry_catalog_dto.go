package delivery

import registryservice "github.com/cylism/cylism-manager/internal/service/registry"

type managedRegistryCatalogPageView struct {
	Repositories []string `json:"repositories"`
	Next         string   `json:"next,omitempty"`
}

type managedRegistryCatalogTagView struct {
	Name          string   `json:"name"`
	Digest        string   `json:"digest"`
	MediaType     string   `json:"media_type,omitempty"`
	Platforms     []string `json:"platforms,omitempty"`
	PullReference string   `json:"pull_reference"`
}

type managedRegistryCatalogTagsPageView struct {
	Repository string                          `json:"repository"`
	Tags       []managedRegistryCatalogTagView `json:"tags"`
	Next       string                          `json:"next,omitempty"`
}

type managedRegistryCatalogReferenceView struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Image string `json:"image"`
}

type managedRegistryCatalogDeletePreflightView struct {
	Repository   string                                `json:"repository"`
	Tag          string                                `json:"tag,omitempty"`
	Digest       string                                `json:"digest,omitempty"`
	AffectedTags []string                              `json:"affected_tags"`
	References   []managedRegistryCatalogReferenceView `json:"references"`
}

func managedRegistryCatalogPageDTO(page *registryservice.CatalogPage) *managedRegistryCatalogPageView {
	if page == nil {
		return nil
	}
	return &managedRegistryCatalogPageView{Repositories: page.Repositories, Next: page.Next}
}

func managedRegistryCatalogTagsPageDTO(page *registryservice.CatalogTagsPage) *managedRegistryCatalogTagsPageView {
	if page == nil {
		return nil
	}
	result := &managedRegistryCatalogTagsPageView{Repository: page.Repository, Next: page.Next, Tags: make([]managedRegistryCatalogTagView, 0, len(page.Tags))}
	for _, tag := range page.Tags {
		result.Tags = append(result.Tags, managedRegistryCatalogTagView{Name: tag.Name, Digest: tag.Digest, MediaType: tag.MediaType, Platforms: tag.Platforms, PullReference: tag.PullReference})
	}
	return result
}

func managedRegistryCatalogDeletePreflightDTO(preflight *registryservice.CatalogDeletePreflight) *managedRegistryCatalogDeletePreflightView {
	if preflight == nil {
		return nil
	}
	result := &managedRegistryCatalogDeletePreflightView{Repository: preflight.Repository, Tag: preflight.Tag, Digest: preflight.Digest, AffectedTags: preflight.AffectedTags, References: make([]managedRegistryCatalogReferenceView, 0, len(preflight.References))}
	for _, reference := range preflight.References {
		result.References = append(result.References, managedRegistryCatalogReferenceView{Kind: reference.Kind, Name: reference.Name, Image: reference.Image})
	}
	return result
}
