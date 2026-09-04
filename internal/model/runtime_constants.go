package model

const (
	RuntimeTypeNanobot = "nanobot"

	RuntimeDeploymentManaged  = "managed"
	RuntimeDeploymentExternal = "external"

	RuntimeStatusDraft       = "draft"
	RuntimeStatusDeploying   = "deploying"
	RuntimeStatusReady       = "ready"
	RuntimeStatusDegraded    = "degraded"
	RuntimeStatusFailed      = "failed"
	RuntimeStatusUninstalled = "uninstalled"
)
