package registry

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
)

// NormalizeImageRegistryEndpoint normalizes a user-provided endpoint for an
// image Registry. Unlike managed Registry endpoints, external image
// Registries may use any valid hostname or host:port combination.
func NormalizeImageRegistryEndpoint(value string) (string, error) {
	endpoint := strings.TrimSuffix(strings.TrimSpace(value), "/")
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")
	if endpoint == "" || strings.ContainsAny(endpoint, " /?#@") || strings.Contains(endpoint, "://") {
		return "", errors.New("镜像仓库地址格式无效")
	}
	return endpoint, nil
}

// VerificationImageReference parses a verification image and ensures it is
// stored in the Registry being configured. The caller owns mismatch wording
// because the managed Registry, image Registry and node-mirror APIs retain
// distinct operator-facing terminology.
func VerificationImageReference(registry, image, mismatchMessage string) (name.Reference, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return nil, errors.New("验证镜像必填")
	}
	ref, err := name.ParseReference(image)
	if err != nil {
		return nil, errors.New("验证镜像格式无效，请填写完整镜像地址和标签")
	}
	if !SameRegistry(ref.Context().RegistryStr(), registry) {
		return nil, errors.New(mismatchMessage)
	}
	return ref, nil
}

// ValidateVerificationImage is the Registry and node-mirror compatibility
// wrapper used by the managed Registry workflow.
func ValidateVerificationImage(registry, image string) error {
	if _, err := name.NewRegistry(strings.TrimSpace(registry)); err != nil {
		return errors.New("Registry 地址无效")
	}
	_, err := VerificationImageReference(registry, image, "验证镜像必须属于当前 Registry")
	return err
}

// MirrorVerificationReference rewrites a source Registry image to the
// configured mirror endpoint while preserving its repository and tag/digest.
func MirrorVerificationReference(registry, image, endpoint string) (name.Reference, error) {
	source, err := VerificationImageReference(registry, image, "验证镜像必须属于当前 Registry")
	if err != nil {
		return nil, err
	}
	target, err := parseHTTPMirrorEndpoint(endpoint)
	if err != nil {
		return nil, errors.New("镜像地址无效")
	}
	if target.Path != "" && target.Path != "/" {
		return nil, errors.New("镜像地址不支持路径")
	}
	options := []name.Option{}
	if target.Scheme == "http" {
		options = append(options, name.Insecure)
	}
	repository, err := name.NewRepository(target.Host+"/"+source.Context().RepositoryStr(), options...)
	if err != nil {
		return nil, errors.New("镜像地址无效")
	}
	switch source := source.(type) {
	case name.Tag:
		return repository.Tag(source.TagStr()), nil
	case name.Digest:
		return repository.Digest(source.DigestStr()), nil
	default:
		return nil, fmt.Errorf("验证镜像格式无效")
	}
}

// ValidateMirrorEndpointURL checks the URL form accepted by K3s mirror
// configuration. Path validation is deferred to verification because existing
// saved configurations may need a more specific diagnostic there.
func ValidateMirrorEndpointURL(endpoint string) error {
	if _, err := parseHTTPMirrorEndpoint(endpoint); err != nil {
		return errors.New("镜像地址必须是 HTTP 或 HTTPS URL")
	}
	return nil
}

func parseHTTPMirrorEndpoint(endpoint string) (*url.URL, error) {
	target, err := url.ParseRequestURI(endpoint)
	if err != nil || target.Host == "" || (target.Scheme != "https" && target.Scheme != "http") {
		return nil, errors.New("invalid mirror endpoint")
	}
	return target, nil
}

// SameRegistry compares Registry hosts using container Registry aliases that
// refer to the same Docker Hub namespace.
func SameRegistry(left, right string) bool {
	normalize := func(value string) string {
		value = strings.ToLower(strings.TrimSpace(value))
		switch value {
		case "index.docker.io", "registry-1.docker.io":
			return "docker.io"
		default:
			return value
		}
	}
	return normalize(left) == normalize(right)
}
