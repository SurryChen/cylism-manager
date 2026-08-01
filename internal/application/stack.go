package application

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// StackSpec describes independent applications composed within one environment.
type StackSpec struct {
	EntryApplication string           `json:"entry_application,omitempty"`
	Components       []StackComponent `json:"components"`
}

type StackComponent struct {
	ApplicationName string      `json:"application_name"`
	Description     string      `json:"description,omitempty"`
	Spec            ReleaseSpec `json:"spec"`
}

func ValidateStackSpec(spec StackSpec) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	if len(spec.Components) == 0 {
		return append(issues, ValidationIssue{Field: "components", Message: "应用栈至少需要一个组件"})
	}
	seen := make(map[string]struct{}, len(spec.Components))
	entryFound := spec.EntryApplication == ""
	for index, component := range spec.Components {
		name := strings.TrimSpace(component.ApplicationName)
		field := fmt.Sprintf("components[%d]", index)
		if len(validation.IsDNS1123Label(name)) > 0 {
			issues = append(issues, ValidationIssue{Field: field + ".application_name", Message: "应用名称必须是小写字母、数字或连字符"})
		}
		if _, exists := seen[name]; exists {
			issues = append(issues, ValidationIssue{Field: field + ".application_name", Message: "应用名称不能重复"})
		}
		seen[name] = struct{}{}
		if name == spec.EntryApplication {
			entryFound = true
		}
		componentIssues := ValidateReleaseSpec(component.Spec)
		for _, issue := range componentIssues {
			issue.Field = field + ".spec." + issue.Field
			issues = append(issues, issue)
		}
	}
	if spec.EntryApplication != "" && (len(validation.IsDNS1123Label(spec.EntryApplication)) > 0 || !entryFound) {
		issues = append(issues, ValidationIssue{Field: "entry_application", Message: "入口应用必须是已定义的组件"})
	}
	return issues
}

// EntryComponent returns the component intended for a separately managed endpoint.
func EntryComponent(spec StackSpec) (StackComponent, bool) {
	for _, component := range spec.Components {
		if component.ApplicationName == spec.EntryApplication {
			return component, true
		}
	}
	return StackComponent{}, false
}
