package model

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	ManagedDocumentResourceSecret    = "secret"
	ManagedDocumentResourceConfigMap = "configmap"
	ManagedDocumentFormatYAML        = "yaml"
	ManagedDocumentFormatJSON        = "json"
)

func NormalizeManagedDocument(kind, format string, paths []string) (string, string, []string, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	format = strings.ToLower(strings.TrimSpace(format))
	if kind != ManagedDocumentResourceSecret && kind != ManagedDocumentResourceConfigMap {
		return "", "", nil, fmt.Errorf("受控文档资源类型必须为 secret 或 configmap")
	}
	if format != ManagedDocumentFormatYAML && format != ManagedDocumentFormatJSON {
		return "", "", nil, fmt.Errorf("受控文档格式必须为 yaml 或 json")
	}
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if !validManagedDocumentPath(path) {
			return "", "", nil, fmt.Errorf("受控文档路径无效")
		}
		seen[path] = struct{}{}
	}
	if len(seen) == 0 {
		return "", "", nil, fmt.Errorf("至少需要一个受控文档路径")
	}
	result := make([]string, 0, len(seen))
	for path := range seen {
		result = append(result, path)
	}
	sort.Strings(result)
	return kind, format, result, nil
}

func (d *ManagedDocument) SetAllowedPaths(paths []string) error {
	_, _, normalized, err := NormalizeManagedDocument(d.ResourceKind, d.Format, paths)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	d.AllowedPaths = string(encoded)
	return nil
}

func (d ManagedDocument) Paths() ([]string, error) {
	var paths []string
	if err := json.Unmarshal([]byte(d.AllowedPaths), &paths); err != nil {
		return nil, fmt.Errorf("解析受控文档路径: %w", err)
	}
	_, _, normalized, err := NormalizeManagedDocument(d.ResourceKind, d.Format, paths)
	return normalized, err
}

func validManagedDocumentPath(path string) bool {
	if path == "" || path == "/" || !strings.HasPrefix(path, "/") || strings.Contains(path, "//") || strings.Contains(path, "..") || strings.Contains(path, "*") {
		return false
	}
	for _, segment := range strings.Split(path[1:], "/") {
		if segment == "" || strings.Contains(segment, "~") {
			return false
		}
	}
	return true
}
