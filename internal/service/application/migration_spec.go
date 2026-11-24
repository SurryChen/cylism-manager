package application

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

var migrationTargetPattern = regexp.MustCompile(`^[0-9]{1,32}$`)
var migrationPrefixPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,47}$`)

func validateMigrationSpec(m *MigrationSpec) []ValidationIssue {
	if m == nil {
		return nil
	}
	var issues []ValidationIssue
	add := func(message string) { issues = append(issues, ValidationIssue{Field: "migration", Message: message}) }
	if m.Target != "" && (!m.Enabled || (!migrationTargetPattern.MatchString(m.Target) || strings.Trim(m.Target, "0") == "")) {
		add("数据库迁移目标必须为 1 至 32 位数字，且模板须启用迁移")
	}
	if !m.Enabled {
		return issues
	}
	if len(m.Command) == 0 || strings.TrimSpace(m.Command[0]) == "" {
		add("请配置镜像内的数据库迁移命令")
	}
	issues = append(issues, validateContainerArguments("migration.command", m.Command)...)
	if !strings.HasPrefix(m.Directory, "/") || path.Clean(m.Directory) != m.Directory {
		add("迁移脚本目录必须为规范的绝对路径")
	}
	if !migrationPrefixPattern.MatchString(m.DatabasePrefix) {
		add("数据库环境变量前缀无效")
	}
	if m.TimeoutSeconds < 30 || m.TimeoutSeconds > 3600 {
		add("迁移超时必须为 30 至 3600 秒")
	}
	return issues
}

func bindMigrationRequest(spec *ReleaseSpec, requests ...*MigrationRequest) error {
	// Templates must never retain a previous release's target.
	if spec.Migration != nil {
		clone := *spec.Migration
		spec.Migration = &clone
		spec.Migration.Target = ""
	}
	var req *MigrationRequest
	if len(requests) > 0 {
		req = requests[0]
	}
	if req == nil {
		if spec.Migration != nil && spec.Migration.Enabled {
			return fmt.Errorf("此模板已启用数据库迁移，请提供 migration.target")
		}
		return nil
	}
	if spec.Migration == nil || !spec.Migration.Enabled {
		return fmt.Errorf("上线模板尚未启用数据库迁移")
	}
	target := strings.TrimSpace(req.Target)
	if !migrationTargetPattern.MatchString(target) || strings.Trim(target, "0") == "" {
		return fmt.Errorf("数据库迁移目标必须为 1 至 32 位数字")
	}
	spec.Migration.Target = target
	if issues := validateMigrationSpec(spec.Migration); len(issues) > 0 {
		return fmt.Errorf("%s", issues[0].Message)
	}
	return nil
}

// Verify that the same enabled variables projected into the Job provide the
// database destination. Secret values themselves are never returned to the UI.
func validateMigrationConnection(spec ReleaseSpec) error {
	if spec.Migration == nil || !spec.Migration.Enabled {
		return nil
	}
	config := enabledStringMap(spec.Config, spec.ConfigDisabled)
	secrets := enabledStringMap(spec.Secrets, spec.SecretsDisabled)
	for _, suffix := range []string{"HOST", "NAME", "USER", "PASSWORD"} {
		key := spec.Migration.DatabasePrefix + "_" + suffix
		value, ok := config[key]
		if v, exists := secrets[key]; exists {
			value, ok = v, true
		}
		if !ok || value == "" {
			return fmt.Errorf("数据库迁移缺少配置 %s", key)
		}
		for _, fileKey := range append(append([]string{}, spec.ConfigFileKeys...), spec.SecretFileKeys...) {
			if fileKey == key {
				return fmt.Errorf("数据库迁移连接 %s 需要作为环境变量注入", key)
			}
		}
	}
	for key := range config {
		if strings.HasPrefix(key, "CYLISM_MIGRATION_") {
			return fmt.Errorf("%s 为数据库迁移保留变量", key)
		}
	}
	for key := range secrets {
		if strings.HasPrefix(key, "CYLISM_MIGRATION_") {
			return fmt.Errorf("%s 为数据库迁移保留变量", key)
		}
	}
	return nil
}
