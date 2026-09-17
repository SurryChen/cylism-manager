package registry

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
)

type catalogClientFake struct {
	repositories []string
	tags         map[string][]string
	manifests    map[string]catalogManifest
	deleted      []string
}

func (f *catalogClientFake) ListRepositories(_ context.Context, last string) ([]string, string, error) {
	if last == "next" {
		return []string{"worker"}, "", nil
	}
	return f.repositories, "next", nil
}
func (f *catalogClientFake) ListTags(_ context.Context, repositoryName, last string) ([]string, string, error) {
	if last != "" {
		return nil, "", nil
	}
	return append([]string(nil), f.tags[repositoryName]...), "", nil
}
func (f *catalogClientFake) Manifest(_ context.Context, repositoryName, reference string) (catalogManifest, error) {
	manifest, ok := f.manifests[repositoryName+":"+reference]
	if !ok {
		return catalogManifest{}, errors.New("manifest not found")
	}
	return manifest, nil
}
func (f *catalogClientFake) DeleteManifest(_ context.Context, repositoryName, digest string) error {
	f.deleted = append(f.deleted, repositoryName+"@"+digest)
	return nil
}

func newCatalogServiceForTest(t *testing.T, references []model.ManagedRegistryContentReference, client *catalogClientFake) *ManagedRegistryCatalogService {
	t.Helper()
	credential, err := EncryptCredential([]byte("01234567890123456789012345678901"), "registry-password")
	if err != nil {
		t.Fatal(err)
	}
	repo := &managedRegistryRepositoryFake{registries: []model.ManagedOCIRegistry{{ID: 7, Endpoint: "registry.example.com", PullUsername: "pull", EncryptedCredential: credential}}}
	service := NewManagedRegistryCatalogService(repo, NewManagedRegistryService(repo, []byte("01234567890123456789012345678901")))
	repo.contentReferences = references
	service.newClient = func(*model.ManagedOCIRegistry, string) managedRegistryCatalogClient { return client }
	return service
}

func TestManagedRegistryCatalogPaginatesAndDoesNotExposeCredentials(t *testing.T) {
	client := &catalogClientFake{repositories: []string{"api"}}
	service := newCatalogServiceForTest(t, nil, client)
	page, err := service.ListRepositories(context.Background(), 7, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Repositories) != 1 || page.Repositories[0] != "api" || page.Next == "" {
		t.Fatalf("unexpected first page: %#v", page)
	}
	next, err := service.ListRepositories(context.Background(), 7, page.Next)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Repositories) != 1 || next.Repositories[0] != "worker" || next.Next != "" {
		t.Fatalf("unexpected second page: %#v", next)
	}
}

func TestManagedRegistryCatalogBlocksSharedDigestReferencedByRelease(t *testing.T) {
	client := &catalogClientFake{tags: map[string][]string{"orders": {"stable", "v1"}}, manifests: map[string]catalogManifest{
		"orders:stable": {Digest: "sha256:shared", MediaType: "application/vnd.oci.image.manifest.v1+json"},
		"orders:v1":     {Digest: "sha256:shared", MediaType: "application/vnd.oci.image.manifest.v1+json"},
	}}
	service := newCatalogServiceForTest(t, []model.ManagedRegistryContentReference{{Kind: "release", Name: "发布 #4", Image: "registry.example.com/orders:stable"}}, client)
	preflight, err := service.PreflightTagDelete(context.Background(), 7, "orders", "stable")
	if err != nil {
		t.Fatal(err)
	}
	if !sameStrings(preflight.AffectedTags, []string{"stable", "v1"}) || len(preflight.References) != 1 {
		t.Fatalf("unexpected preflight: %#v", preflight)
	}
	_, err = service.DeleteTag(context.Background(), 7, *preflight)
	if !errors.Is(err, ErrContentReferenced) || len(client.deleted) != 0 {
		t.Fatalf("referenced manifest must not be deleted: %v %#v", err, client.deleted)
	}
}

func TestManagedRegistryCatalogRejectsStaleDeleteConfirmation(t *testing.T) {
	client := &catalogClientFake{tags: map[string][]string{"orders": {"v1"}}, manifests: map[string]catalogManifest{"orders:v1": {Digest: "sha256:one"}}}
	service := newCatalogServiceForTest(t, nil, client)
	_, err := service.DeleteTag(context.Background(), 7, CatalogDeletePreflight{Repository: "orders", Tag: "v1", Digest: "sha256:stale", AffectedTags: []string{"v1"}})
	if !errors.Is(err, ErrCatalogChanged) || len(client.deleted) != 0 {
		t.Fatalf("stale confirmation must not delete: %v %#v", err, client.deleted)
	}
}

func TestDistributionCatalogClientPreservesNestedRepositoryPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v2/team/orders/tags/list" {
			t.Errorf("path = %q", request.URL.Path)
		}
		username, password, ok := request.BasicAuth()
		if !ok || username != "pull" || password != "registry-password" {
			t.Error("missing managed registry Basic Auth")
		}
		_, _ = w.Write([]byte(`{"name":"team/orders","tags":["v1"]}`))
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := newDistributionCatalogClient(&model.ManagedOCIRegistry{Endpoint: endpoint.Host, InsecureHTTP: true, PullUsername: "pull"}, "registry-password")
	tags, _, err := client.ListTags(context.Background(), "team/orders", "")
	if err != nil || len(tags) != 1 || tags[0] != "v1" {
		t.Fatalf("ListTags = %#v, %v", tags, err)
	}
}
