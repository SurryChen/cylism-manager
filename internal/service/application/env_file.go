package application

import (
	"fmt"
	"regexp"
	"strings"
)

// EnvFileField describes an assignment without disclosing its value.
type EnvFileField struct {
	Line int    `json:"line"`
	Name string `json:"name"`
}

// A nil value preserves the original assignment, including its quoting and comments.
// Line -1 adds a new assignment. Existing assignments are addressed by line and name.
type EnvFileChange struct {
	Line   int     `json:"line"`
	Name   string  `json:"name"`
	Value  *string `json:"value,omitempty"`
	Remove bool    `json:"remove,omitempty"`
}

var envAssignment = regexp.MustCompile(`^(\s*(?:export\s+)?)([A-Za-z_][A-Za-z0-9_]*)(\s*=\s*)(.*)$`)
var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func inspectEnvLines(content string) ([]string, []EnvFileField, error) {
	if len(content) > 1<<20 {
		return nil, nil, fmt.Errorf("文件过大，请使用全文编辑")
	}
	lines := strings.Split(content, "\n")
	fields := make([]EnvFileField, 0)
	names := make(map[string]bool)
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := envAssignment.FindStringSubmatch(strings.TrimSuffix(line, "\r"))
		if parts == nil || names[parts[2]] || !singleLineEnvValue(parts[4]) {
			return nil, nil, fmt.Errorf("文件包含重复配置项、多行值或非标准赋值，请使用全文编辑")
		}
		names[parts[2]] = true
		fields = append(fields, EnvFileField{Line: index, Name: parts[2]})
	}
	return lines, fields, nil
}

func singleLineEnvValue(value string) bool {
	if strings.ContainsAny(value, "\r\n\x00") || strings.HasSuffix(strings.TrimSpace(value), `\`) {
		return false
	}
	value = strings.TrimSpace(value)
	if value == "" || (value[0] != '\'' && value[0] != '"' && value[0] != '`') {
		return true
	}
	quote := value[0]
	for index := 1; index < len(value); index++ {
		if quote != '\'' && value[index] == '\\' {
			index++
			continue
		}
		if value[index] == quote {
			tail := strings.TrimSpace(value[index+1:])
			return tail == "" || strings.HasPrefix(tail, "#")
		}
	}
	return false
}

func InspectEnvFile(content string) ([]EnvFileField, error) {
	_, fields, err := inspectEnvLines(content)
	return fields, err
}

// ReadEnvFileField returns only the requested assignment after checking its identity.
func ReadEnvFileField(content string, line int, name string) (string, error) {
	lines, fields, err := inspectEnvLines(content)
	if err != nil {
		return "", err
	}
	for _, field := range fields {
		if field.Line == line && field.Name == name {
			parts := envAssignment.FindStringSubmatch(strings.TrimSuffix(lines[line], "\r"))
			return parts[4], nil
		}
	}
	return "", fmt.Errorf("配置项已改变或不存在，请重新读取文件")
}

func UpdateEnvFile(content string, changes []EnvFileChange) (string, error) {
	lines, fields, err := inspectEnvLines(content)
	if err != nil {
		return "", err
	}
	byLine := make(map[int]EnvFileField, len(fields))
	names := make(map[string]bool, len(fields))
	for _, field := range fields {
		byLine[field.Line], names[field.Name] = field, true
	}
	touched, removed := make(map[int]bool), make(map[int]bool)
	ending := ""
	if strings.Contains(content, "\r\n") {
		ending = "\r"
	}
	var additions []string
	for _, change := range changes {
		if !envName.MatchString(change.Name) || (change.Value != nil && !singleLineEnvValue(*change.Value)) {
			return "", fmt.Errorf("配置项名称或值无效，多行内容请使用全文编辑")
		}
		if change.Line == -1 {
			if change.Remove || change.Value == nil || names[change.Name] {
				return "", fmt.Errorf("新增配置项无效或名称重复")
			}
			names[change.Name] = true
			additions = append(additions, change.Name+"="+*change.Value+ending)
			continue
		}
		field, exists := byLine[change.Line]
		if !exists || field.Name != change.Name || touched[change.Line] || (change.Remove && change.Value != nil) {
			return "", fmt.Errorf("配置项已改变或重复，请重新读取文件")
		}
		touched[change.Line] = true
		if change.Remove {
			removed[change.Line] = true
		} else if change.Value != nil {
			parts := envAssignment.FindStringSubmatch(strings.TrimSuffix(lines[change.Line], "\r"))
			lines[change.Line] = parts[1] + parts[2] + parts[3] + *change.Value + ending
		}
	}
	result := make([]string, 0, len(lines)+len(additions))
	for index, line := range lines {
		if !removed[index] {
			result = append(result, line)
		}
	}
	if len(additions) > 0 {
		if len(result) > 0 && result[len(result)-1] == "" {
			result = result[:len(result)-1]
		}
		result = append(result, additions...)
		result = append(result, "")
	}
	updated := strings.Join(result, "\n")
	if len(updated) > 1<<20 {
		return "", fmt.Errorf("配置文件不能超过 1 MiB")
	}
	return updated, nil
}
