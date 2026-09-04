package application

import (
	"sort"
	"strings"
)

func SanitizeReleaseSpec(spec ReleaseSpec) ReleaseSpec {
	spec.RegistryEndpoint = ""
	spec.RegistryAuthType = ""
	spec.RegistryUsername = ""
	spec.RegistryCredential = ""
	spec.ImageVerificationEndpoint = ""
	spec.ImageVerificationUsername = ""
	spec.ImageVerificationCredential = ""
	spec.ImageVerificationInsecureSkipVerify = false
	sanitized := spec
	sanitized.Config = cloneStringMap(spec.Config)
	sanitized.Secrets = make(map[string]string, len(spec.Secrets))
	for key := range spec.Secrets {
		sanitized.Secrets[key] = ""
	}
	return sanitized
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func enabledStringMap(values map[string]string, disabled []string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	disabledSet := make(map[string]struct{}, len(disabled))
	for _, key := range disabled {
		if key = strings.TrimSpace(key); key != "" {
			disabledSet[key] = struct{}{}
		}
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		if _, isDisabled := disabledSet[key]; !isDisabled {
			result[key] = value
		}
	}
	return result
}

func hasSecretValues(values map[string]string) bool {
	for _, value := range values {
		if value != "" {
			return true
		}
	}
	return false
}

func mergeLabels(maps ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, labels := range maps {
		for key, value := range labels {
			merged[key] = value
		}
	}
	return merged
}

func SortedSecretKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
