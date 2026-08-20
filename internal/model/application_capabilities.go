package model

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const maxApplicationCapabilities = 16

var applicationCapabilityPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)

// NormalizeApplicationCapabilities returns the stable API representation used
// for persisted application metadata. Capabilities are opaque to the platform.
func NormalizeApplicationCapabilities(values []string) ([]string, error) {
	if len(values) > maxApplicationCapabilities {
		return nil, fmt.Errorf("能力标签最多 %d 项", maxApplicationCapabilities)
	}
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		capability := strings.ToLower(strings.TrimSpace(value))
		if !applicationCapabilityPattern.MatchString(capability) {
			return nil, fmt.Errorf("能力标签 %q 无效，只能使用小写字母、数字和连字符，长度不超过 64", value)
		}
		unique[capability] = struct{}{}
	}
	result := make([]string, 0, len(unique))
	for capability := range unique {
		result = append(result, capability)
	}
	sort.Strings(result)
	return result, nil
}

// SetCapabilities normalizes the public list before the store persists it.
func (application *Application) SetCapabilities(values []string) error {
	normalized, err := NormalizeApplicationCapabilities(values)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	application.Capabilities = normalized
	application.CapabilitiesData = string(encoded)
	return nil
}

// LoadCapabilities decodes older empty database values as an empty list.
func (application *Application) LoadCapabilities() {
	application.Capabilities = []string{}
	if strings.TrimSpace(application.CapabilitiesData) == "" {
		return
	}
	var values []string
	if err := json.Unmarshal([]byte(application.CapabilitiesData), &values); err != nil {
		return
	}
	normalized, err := NormalizeApplicationCapabilities(values)
	if err == nil {
		application.Capabilities = normalized
	}
}
