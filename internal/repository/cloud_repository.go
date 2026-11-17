package repository

import (
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
)

type CloudConnectionRepository interface {
	CreateCloudConnection(*model.CloudConnection) error
	ListCloudConnections() ([]model.CloudConnection, error)
	ListCloudConnectionsPage(int, int) ([]model.CloudConnection, int64, error)
	GetCloudConnection(uint) (*model.CloudConnection, error)
	UpdateCloudConnection(*model.CloudConnection) error
	DeleteCloudConnection(uint) error
}

var _ CloudConnectionRepository = (*store.Store)(nil)
