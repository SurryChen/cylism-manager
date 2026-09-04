package application

import (
	"github.com/cylism/cylism-manager/internal/repository"
)

// ReleaseService is kept as the composition-facing name for the release
// workflow. The implementation now lives directly in this service package;
// this alias avoids an extra forwarding object.
type ReleaseService = ReleaseWorkflow

func NewReleaseService(repository repository.ReleaseWorkflowRepository, encKey []byte, applier ResourceApplier) *ReleaseService {
	return NewReleaseWorkflow(repository, encKey, applier)
}
