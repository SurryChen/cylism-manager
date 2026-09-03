package maintenance

import (
	"context"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

// SSHExecutor is the transport boundary used by node maintenance services.
// Implementations belong to infrastructure; maintenance only knows how to
// execute a bounded command on a managed node.
type SSHExecutor interface {
	Execute(context.Context, time.Duration, *model.Server, string) ([]byte, error)
}

type SSHExecutorFunc func(context.Context, time.Duration, *model.Server, string) ([]byte, error)

func (f SSHExecutorFunc) Execute(ctx context.Context, timeout time.Duration, server *model.Server, command string) ([]byte, error) {
	return f(ctx, timeout, server, command)
}

type Inspector interface {
	Inspect(context.Context, *model.Server) (Inspection, error)
}

type InspectorFunc func(context.Context, *model.Server) (Inspection, error)

func (f InspectorFunc) Inspect(ctx context.Context, server *model.Server) (Inspection, error) {
	return f(ctx, server)
}

type CleanupExecutor interface {
	Execute(context.Context, *model.Server, string) (string, error)
}

type CleanupExecutorFunc func(context.Context, *model.Server, string) (string, error)

func (f CleanupExecutorFunc) Execute(ctx context.Context, server *model.Server, recipe string) (string, error) {
	return f(ctx, server, recipe)
}

type Inspection struct {
	Node           string            `json:"node"`
	Filesystems    []FilesystemUsage `json:"filesystems"`
	JournalBytes   int64             `json:"journal_bytes,omitempty"`
	JournalSummary string            `json:"journal_summary,omitempty"`
	Directories    []DirectoryUsage  `json:"directories"`
}

type FilesystemUsage struct {
	MountPoint     string `json:"mount_point"`
	SizeBytes      int64  `json:"size_bytes"`
	UsedBytes      int64  `json:"used_bytes"`
	AvailableBytes int64  `json:"available_bytes"`
	UsedPercent    int    `json:"used_percent"`
}

type DirectoryUsage struct {
	Path    string       `json:"path"`
	Entries []UsageEntry `json:"entries"`
}

type UsageEntry struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

func ValidCleanupRecipe(recipe string) bool {
	return recipe == "journal-vacuum" || recipe == "container-image-prune" || recipe == "docker-image-prune"
}

func CleanupSummary(node, recipe string) string {
	switch recipe {
	case "journal-vacuum":
		return "vacuum system journal older than 7 days on node " + node
	case "docker-image-prune":
		return "prune unused Docker images on node " + node
	default:
		return "prune unused container images on node " + node
	}
}
