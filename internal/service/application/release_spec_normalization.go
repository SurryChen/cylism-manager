package application

import (
	"strings"
)

func NormalizeManagedKeys(spec *ReleaseSpec) {
	if len(spec.ConfigManagedKeys) == 0 {
		for _, mount := range spec.FileMounts {
			if mount.Managed && mount.SourceType == FileMountSourceApplicationConfig && mount.Key != "" {
				spec.ConfigManagedKeys = appendUniqueKey(spec.ConfigManagedKeys, mount.Key)
			}
		}
	}
	if len(spec.SecretManagedKeys) == 0 {
		for _, mount := range spec.FileMounts {
			if mount.Managed && mount.SourceType == FileMountSourceApplicationSecret && mount.Key != "" {
				spec.SecretManagedKeys = appendUniqueKey(spec.SecretManagedKeys, mount.Key)
			}
		}
	}
}

func appendUniqueKey(keys []string, key string) []string {
	key = strings.TrimSpace(key)
	for _, existing := range keys {
		if existing == key {
			return keys
		}
	}
	return append(keys, key)
}

func IsConfigKeyManaged(spec ReleaseSpec, key string) bool {
	if _, exists := spec.Config[key]; !exists {
		return false
	}
	if len(spec.ConfigManagedKeys) > 0 {
		return containsKey(spec.ConfigManagedKeys, key)
	}
	for _, mount := range spec.FileMounts {
		if mount.Managed && mount.SourceType == FileMountSourceApplicationConfig && mount.Key == key {
			return true
		}
	}
	return false
}

func IsSecretKeyManaged(spec ReleaseSpec, key string) bool {
	if _, exists := spec.Secrets[key]; !exists {
		return false
	}
	if len(spec.SecretManagedKeys) > 0 {
		return containsKey(spec.SecretManagedKeys, key)
	}
	for _, mount := range spec.FileMounts {
		if mount.Managed && mount.SourceType == FileMountSourceApplicationSecret && mount.Key == key {
			return true
		}
	}
	return false
}

func containsKey(keys []string, key string) bool {
	for _, candidate := range keys {
		if strings.TrimSpace(candidate) == key {
			return true
		}
	}
	return false
}

func normalizeServiceProtocol(protocol string) string {
	switch strings.ToUpper(strings.TrimSpace(protocol)) {
	case "", ServiceProtocolTCP:
		return ServiceProtocolTCP
	case ServiceProtocolUDP:
		return ServiceProtocolUDP
	default:
		return ""
	}
}

func normalizeServiceType(serviceType string) string {
	switch strings.ToLower(strings.TrimSpace(serviceType)) {
	case "", "clusterip":
		return ServiceTypeClusterIP
	case "nodeport":
		return ServiceTypeNodePort
	case "loadbalancer":
		return ServiceTypeLoadBalancer
	default:
		return ""
	}
}
