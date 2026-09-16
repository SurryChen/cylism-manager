package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/repository"
)

const managedRegistryCatalogPageSize = 100
const managedRegistryCatalogMetadataConcurrency = 8

var (
	ErrCatalogUnavailable = errors.New("managed registry catalog is unavailable")
	ErrCatalogChanged     = errors.New("managed registry catalog changed")
	ErrContentReferenced  = errors.New("managed registry content is referenced")
)

type CatalogPage struct {
	Repositories []string
	Next         string
}

type CatalogTag struct {
	Name          string
	Digest        string
	MediaType     string
	Platforms     []string
	PullReference string
}

type CatalogTagsPage struct {
	Repository string
	Tags       []CatalogTag
	Next       string
}

type CatalogReference struct {
	Kind  string
	Name  string
	Image string
}

type CatalogDeletePreflight struct {
	Repository   string
	Tag          string
	Digest       string
	AffectedTags []string
	References   []CatalogReference
}

type catalogManifest struct {
	Digest    string
	MediaType string
	Platforms []string
}

// managedRegistryCatalogClient isolates the OCI Distribution transport from
// policy decisions and allows service tests to exercise unsafe states without
// a running Registry.
type managedRegistryCatalogClient interface {
	ListRepositories(context.Context, string) ([]string, string, error)
	ListTags(context.Context, string, string) ([]string, string, error)
	Manifest(context.Context, string, string) (catalogManifest, error)
	DeleteManifest(context.Context, string, string) error
}

type catalogClientFactory func(*model.ManagedOCIRegistry, string) managedRegistryCatalogClient

type ManagedRegistryCatalogService struct {
	referenceStore repository.ManagedRegistryCatalogRepository
	managed        *ManagedRegistryService
	newClient      catalogClientFactory
}

func NewManagedRegistryCatalogService(referenceStore repository.ManagedRegistryCatalogRepository, managed *ManagedRegistryService) *ManagedRegistryCatalogService {
	return &ManagedRegistryCatalogService{referenceStore: referenceStore, managed: managed, newClient: newDistributionCatalogClient}
}

func (s *ManagedRegistryCatalogService) ListRepositories(ctx context.Context, registryID uint, continuation string) (*CatalogPage, error) {
	client, err := s.client(registryID)
	if err != nil {
		return nil, err
	}
	last, err := decodeContinuation(continuation)
	if err != nil {
		return nil, err
	}
	repositories, next, err := client.ListRepositories(ctx, last)
	if err != nil {
		return nil, catalogError(err)
	}
	sort.Strings(repositories)
	return &CatalogPage{Repositories: repositories, Next: encodeContinuation(next)}, nil
}

func (s *ManagedRegistryCatalogService) ListTags(ctx context.Context, registryID uint, repositoryName, continuation string) (*CatalogTagsPage, error) {
	if err := validateRepository(repositoryName); err != nil {
		return nil, err
	}
	client, err := s.client(registryID)
	if err != nil {
		return nil, err
	}
	last, err := decodeContinuation(continuation)
	if err != nil {
		return nil, err
	}
	tags, next, err := client.ListTags(ctx, repositoryName, last)
	if err != nil {
		return nil, catalogError(err)
	}
	manifests, err := resolveTagManifests(ctx, client, repositoryName, tags)
	if err != nil {
		return nil, catalogError(err)
	}
	result := make([]CatalogTag, 0, len(tags))
	for _, tag := range tags {
		manifest := manifests[tag]
		result = append(result, CatalogTag{Name: tag, Digest: manifest.Digest, MediaType: manifest.MediaType, Platforms: manifest.Platforms, PullReference: repositoryName + ":" + tag})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return &CatalogTagsPage{Repository: repositoryName, Tags: result, Next: encodeContinuation(next)}, nil
}

func (s *ManagedRegistryCatalogService) PreflightTagDelete(ctx context.Context, registryID uint, repositoryName, tag string) (*CatalogDeletePreflight, error) {
	if err := validateRepository(repositoryName); err != nil || strings.TrimSpace(tag) == "" {
		return nil, errors.New("仓库或标签无效")
	}
	client, err := s.client(registryID)
	if err != nil {
		return nil, err
	}
	manifest, err := client.Manifest(ctx, repositoryName, tag)
	if err != nil {
		return nil, catalogError(err)
	}
	affected, err := s.tagsForDigest(ctx, client, repositoryName, manifest.Digest)
	if err != nil {
		return nil, err
	}
	registryEndpoint, err := s.registryEndpoint(registryID)
	if err != nil {
		return nil, err
	}
	references, err := s.references(registryEndpoint, repositoryName, manifest.Digest, affected)
	if err != nil {
		return nil, err
	}
	return &CatalogDeletePreflight{Repository: repositoryName, Tag: tag, Digest: manifest.Digest, AffectedTags: affected, References: references}, nil
}

func (s *ManagedRegistryCatalogService) DeleteTag(ctx context.Context, registryID uint, expected CatalogDeletePreflight) (*CatalogDeletePreflight, error) {
	if strings.TrimSpace(expected.Tag) == "" || strings.TrimSpace(expected.Digest) == "" {
		return nil, errors.New("删除确认信息不完整")
	}
	current, err := s.PreflightTagDelete(ctx, registryID, expected.Repository, expected.Tag)
	if err != nil {
		return nil, err
	}
	if current.Digest != expected.Digest || !sameStrings(current.AffectedTags, expected.AffectedTags) {
		return nil, ErrCatalogChanged
	}
	if len(current.References) > 0 {
		return current, ErrContentReferenced
	}
	client, err := s.client(registryID)
	if err != nil {
		return nil, err
	}
	if err := client.DeleteManifest(ctx, current.Repository, current.Digest); err != nil {
		return nil, catalogError(err)
	}
	return current, nil
}

func (s *ManagedRegistryCatalogService) PreflightRepositoryDelete(ctx context.Context, registryID uint, repositoryName string) (*CatalogDeletePreflight, error) {
	if err := validateRepository(repositoryName); err != nil {
		return nil, err
	}
	client, err := s.client(registryID)
	if err != nil {
		return nil, err
	}
	tags, err := allTags(ctx, client, repositoryName)
	if err != nil {
		return nil, err
	}
	manifests, err := resolveTagManifests(ctx, client, repositoryName, tags)
	if err != nil {
		return nil, catalogError(err)
	}
	digests := map[string][]string{}
	for _, tag := range tags {
		manifest := manifests[tag]
		digests[manifest.Digest] = append(digests[manifest.Digest], tag)
	}
	allAffected := append([]string(nil), tags...)
	sort.Strings(allAffected)
	registryEndpoint, err := s.registryEndpoint(registryID)
	if err != nil {
		return nil, err
	}
	allReferences := make([]CatalogReference, 0)
	for digest, affected := range digests {
		references, err := s.references(registryEndpoint, repositoryName, digest, affected)
		if err != nil {
			return nil, err
		}
		allReferences = append(allReferences, references...)
	}
	return &CatalogDeletePreflight{Repository: repositoryName, AffectedTags: allAffected, References: dedupeReferences(allReferences)}, nil
}

func (s *ManagedRegistryCatalogService) DeleteRepository(ctx context.Context, registryID uint, expected CatalogDeletePreflight) (*CatalogDeletePreflight, error) {
	if strings.TrimSpace(expected.Repository) == "" {
		return nil, errors.New("删除确认信息不完整")
	}
	current, err := s.PreflightRepositoryDelete(ctx, registryID, expected.Repository)
	if err != nil {
		return nil, err
	}
	if !sameStrings(current.AffectedTags, expected.AffectedTags) {
		return nil, ErrCatalogChanged
	}
	if len(current.References) > 0 {
		return current, ErrContentReferenced
	}
	client, err := s.client(registryID)
	if err != nil {
		return nil, err
	}
	manifests, err := resolveTagManifests(ctx, client, current.Repository, current.AffectedTags)
	if err != nil {
		return nil, ErrCatalogChanged
	}
	digests := map[string]struct{}{}
	for _, tag := range current.AffectedTags {
		manifest := manifests[tag]
		digests[manifest.Digest] = struct{}{}
	}
	for digest := range digests {
		if err := client.DeleteManifest(ctx, current.Repository, digest); err != nil {
			return nil, catalogError(err)
		}
	}
	return current, nil
}

func (s *ManagedRegistryCatalogService) client(registryID uint) (managedRegistryCatalogClient, error) {
	if s == nil || s.managed == nil || s.newClient == nil {
		return nil, errors.New("制品库目录服务未初始化")
	}
	registry, err := s.managed.Get(registryID)
	if err != nil {
		return nil, err
	}
	password, err := s.managed.ResolveStoredPassword(registry)
	if err != nil {
		return nil, err
	}
	return s.newClient(registry, password), nil
}

func (s *ManagedRegistryCatalogService) tagsForDigest(ctx context.Context, client managedRegistryCatalogClient, repositoryName, digest string) ([]string, error) {
	tags, err := allTags(ctx, client, repositoryName)
	if err != nil {
		return nil, err
	}
	manifests, err := resolveTagManifests(ctx, client, repositoryName, tags)
	if err != nil {
		return nil, catalogError(err)
	}
	affected := make([]string, 0, len(tags))
	for _, item := range tags {
		manifest := manifests[item]
		if manifest.Digest == digest {
			affected = append(affected, item)
		}
	}
	sort.Strings(affected)
	return affected, nil
}

func (s *ManagedRegistryCatalogService) references(registryEndpoint, repositoryName, digest string, tags []string) ([]CatalogReference, error) {
	items, err := s.referenceStore.ListManagedRegistryContentReferences()
	if err != nil {
		return nil, fmt.Errorf("读取制品库引用失败: %w", err)
	}
	result := make([]CatalogReference, 0)
	for _, item := range items {
		if managedReferenceMatches(registryEndpoint, repositoryName, digest, tags, item) {
			result = append(result, CatalogReference{Kind: item.Kind, Name: item.Name, Image: item.Image})
		}
	}
	return dedupeReferences(result), nil
}

func (s *ManagedRegistryCatalogService) registryEndpoint(registryID uint) (string, error) {
	registry, err := s.managed.Get(registryID)
	if err != nil {
		return "", err
	}
	return strings.Trim(strings.TrimSpace(registry.Endpoint), "/"), nil
}

func allTags(ctx context.Context, client managedRegistryCatalogClient, repositoryName string) ([]string, error) {
	last, tags := "", []string{}
	for {
		page, next, err := client.ListTags(ctx, repositoryName, last)
		if err != nil {
			return nil, catalogError(err)
		}
		tags = append(tags, page...)
		if next == "" {
			break
		}
		last = next
	}
	sort.Strings(tags)
	return tags, nil
}

func resolveTagManifests(ctx context.Context, client managedRegistryCatalogClient, repositoryName string, tags []string) (map[string]catalogManifest, error) {
	if len(tags) == 0 {
		return map[string]catalogManifest{}, nil
	}
	type result struct {
		tag      string
		manifest catalogManifest
		err      error
	}
	jobs := make(chan string)
	results := make(chan result, len(tags))
	workers := managedRegistryCatalogMetadataConcurrency
	if len(tags) < workers {
		workers = len(tags)
	}
	var group sync.WaitGroup
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for tag := range jobs {
				manifest, err := client.Manifest(ctx, repositoryName, tag)
				results <- result{tag: tag, manifest: manifest, err: err}
			}
		}()
	}
	go func() {
		for _, tag := range tags {
			jobs <- tag
		}
		close(jobs)
		group.Wait()
		close(results)
	}()
	manifests := make(map[string]catalogManifest, len(tags))
	for item := range results {
		if item.err != nil {
			return nil, item.err
		}
		manifests[item.tag] = item.manifest
	}
	return manifests, nil
}

func decodeContinuation(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) == 0 {
		return "", errors.New("目录分页标记无效")
	}
	return string(decoded), nil
}
func encodeContinuation(value string) string {
	if value == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}
func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	a, b := append([]string(nil), left...), append([]string(nil), right...)
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func validateRepository(value string) error {
	value = strings.Trim(value, "/")
	if value == "" || strings.ContainsAny(value, " \t\r\n") || strings.Contains(value, "..") {
		return errors.New("仓库名称无效")
	}
	return nil
}
func catalogError(err error) error {
	if errors.Is(err, ErrCatalogChanged) || errors.Is(err, ErrContentReferenced) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrCatalogUnavailable, err)
}

func dedupeReferences(items []CatalogReference) []CatalogReference {
	seen, result := map[string]struct{}{}, make([]CatalogReference, 0, len(items))
	for _, item := range items {
		key := item.Kind + "\\x00" + item.Name + "\\x00" + item.Image
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Kind+result[i].Name < result[j].Kind+result[j].Name })
	return result
}

func managedReferenceMatches(endpoint, repositoryName, digest string, tags []string, item model.ManagedRegistryContentReference) bool {
	image := strings.Trim(strings.TrimSpace(item.Image), "/")
	if image == "" {
		return item.Kind == "template"
	} // malformed persisted template: conservatively block deletion.
	path := image
	if strings.HasPrefix(path, endpoint+"/") {
		path = strings.TrimPrefix(path, endpoint+"/")
	} else if item.Kind == "template" && !looksLikeRegistry(path) { /* default managed Registry template */
	} else {
		return false
	}
	repository, imageTag, imageDigest := splitImagePath(path)
	if repository != repositoryName {
		return false
	}
	if item.Digest == digest || imageDigest == digest {
		return true
	}
	if imageTag == "" {
		return item.Kind == "template"
	}
	for _, tag := range tags {
		if imageTag == tag {
			return true
		}
	}
	return false
}
func looksLikeRegistry(value string) bool {
	first := strings.Split(value, "/")[0]
	return strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost"
}
func splitImagePath(value string) (repository, tag, digest string) {
	if at := strings.Index(value, "@"); at >= 0 {
		return value[:at], "", value[at+1:]
	}
	slash := strings.LastIndex(value, "/")
	colon := strings.LastIndex(value, ":")
	if colon > slash {
		return value[:colon], value[colon+1:], ""
	}
	return value, "", ""
}

type distributionCatalogClient struct {
	base               *url.URL
	username, password string
	http               *http.Client
}

func newDistributionCatalogClient(registry *model.ManagedOCIRegistry, password string) managedRegistryCatalogClient {
	scheme := "https"
	if registry.InsecureHTTP {
		scheme = "http"
	}
	return &distributionCatalogClient{base: &url.URL{Scheme: scheme, Host: registry.Endpoint}, username: registry.PullUsername, password: password, http: &http.Client{Timeout: 15 * time.Second}}
}
func (c *distributionCatalogClient) ListRepositories(ctx context.Context, last string) ([]string, string, error) {
	var response struct {
		Repositories []string `json:"repositories"`
	}
	next, err := c.getJSON(ctx, "/v2/_catalog", last, &response)
	return response.Repositories, next, err
}
func (c *distributionCatalogClient) ListTags(ctx context.Context, repositoryName, last string) ([]string, string, error) {
	var response struct {
		Tags []string `json:"tags"`
	}
	next, err := c.getJSON(ctx, "/v2/"+registryPath(repositoryName)+"/tags/list", last, &response)
	return response.Tags, next, err
}
func (c *distributionCatalogClient) Manifest(ctx context.Context, repositoryName, reference string) (catalogManifest, error) {
	request, err := c.request(ctx, http.MethodGet, "/v2/"+registryPath(repositoryName)+"/manifests/"+url.PathEscape(reference))
	if err != nil {
		return catalogManifest{}, err
	}
	request.Header.Set("Accept", "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json")
	response, err := c.http.Do(request)
	if err != nil {
		return catalogManifest{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return catalogManifest{}, fmt.Errorf("Registry 返回 %d", response.StatusCode)
	}
	result := catalogManifest{Digest: response.Header.Get("Docker-Content-Digest"), MediaType: response.Header.Get("Content-Type")}
	if result.Digest == "" {
		return catalogManifest{}, errors.New("Registry 未返回 manifest digest")
	}
	var raw struct {
		Manifests []struct {
			Platform struct {
				OS           string `json:"os"`
				Architecture string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&raw) == nil {
		for _, item := range raw.Manifests {
			if item.Platform.OS != "" && item.Platform.Architecture != "" {
				result.Platforms = append(result.Platforms, item.Platform.OS+"/"+item.Platform.Architecture)
			}
		}
	}
	return result, nil
}
func (c *distributionCatalogClient) DeleteManifest(ctx context.Context, repositoryName, digest string) error {
	request, err := c.request(ctx, http.MethodDelete, "/v2/"+registryPath(repositoryName)+"/manifests/"+url.PathEscape(digest))
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Registry 返回 %d", response.StatusCode)
	}
	return nil
}
func (c *distributionCatalogClient) getJSON(ctx context.Context, path, last string, target any) (string, error) {
	request, err := c.request(ctx, http.MethodGet, path)
	if err != nil {
		return "", err
	}
	query := request.URL.Query()
	query.Set("n", fmt.Sprint(managedRegistryCatalogPageSize))
	if last != "" {
		query.Set("last", last)
	}
	request.URL.RawQuery = query.Encode()
	response, err := c.http.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("Registry 返回 %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(target); err != nil {
		return "", err
	}
	return nextLast(response.Header.Get("Link")), nil
}
func (c *distributionCatalogClient) request(ctx context.Context, method, path string) (*http.Request, error) {
	target := *c.base
	target.Path = path
	request, err := http.NewRequestWithContext(ctx, method, target.String(), nil)
	if err != nil {
		return nil, err
	}
	request.SetBasicAuth(c.username, c.password)
	return request, nil
}
func registryPath(value string) string {
	return strings.Trim(value, "/")
}
func nextLast(link string) string {
	if link == "" || !strings.Contains(link, "rel=\"next\"") {
		return ""
	}
	start, end := strings.Index(link, "<"), strings.Index(link, ">")
	if start < 0 || end <= start {
		return ""
	}
	target, err := url.Parse(link[start+1 : end])
	if err != nil {
		return ""
	}
	return target.Query().Get("last")
}
