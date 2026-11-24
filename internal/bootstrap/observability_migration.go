package bootstrap

import (
	"context"
	"github.com/cylism/cylism-manager/internal/model"
	migration "github.com/cylism/cylism-manager/internal/service/observability/migration"
	"github.com/cylism/cylism-manager/internal/transport"
	"time"
)

type observabilityTransfer struct{ key []byte }

func (t observabilityTransfer) Exec(ctx context.Context, server *model.Server, command string) ([]byte, error) {
	return transport.SSHExecServerContext(ctx, 10*time.Minute, server, t.key, command)
}
func (t observabilityTransfer) Copy(ctx context.Context, source, target *model.Server, sourcePath, targetPath string, control migration.CopyControl) (int64, error) {
	return transport.CopyDirectoryControlledContext(ctx, source, target, sourcePath, targetPath, t.key, control)
}

func (c *Container) configureObservabilityMigrations(factory *clusterAgentClientFactory) {
	if c.migrations != nil || c.Store == nil {
		return
	}
	c.migrations = migration.New(c.Store, func(ctx context.Context, clusterID uint) (migration.Adapter, error) {
		return factory.Client(ctx, clusterID)
	}, observabilityTransfer{key: c.configEncryptionKey()})
}
