package model

import "fmt"

// NamespaceConflictError indicates that another environment already owns a Namespace.
type NamespaceConflictError struct {
	Namespace     string
	EnvironmentID uint
	ProjectID     uint
}

func (e *NamespaceConflictError) Error() string {
	return fmt.Sprintf("命名空间 %q 已被项目 %d 的环境 %d 绑定", e.Namespace, e.ProjectID, e.EnvironmentID)
}

// NamespaceConflict groups legacy duplicate bindings for an operator-directed migration.
type NamespaceConflict struct {
	Namespace    string        `json:"namespace"`
	Environments []Environment `json:"environments"`
}

// TemplateRevisionConflictError indicates that a template changed after a caller read it.
type TemplateRevisionConflictError struct {
	Current  uint
	Expected uint
}

func (e *TemplateRevisionConflictError) Error() string {
	return fmt.Sprintf("模板版本冲突，当前版本为 %d，期望版本为 %d", e.Current, e.Expected)
}
