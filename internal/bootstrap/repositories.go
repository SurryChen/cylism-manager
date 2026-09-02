package bootstrap

import "github.com/cylism/cylism-manager/internal/store"

// NewRepositories opens the platform persistence implementation. Repository
// interfaces are consumed by services; the bootstrap layer only owns the
// concrete Store lifetime.
func NewRepositories(path string) (*store.Store, error) { return store.New(path) }
