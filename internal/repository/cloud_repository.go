package repository

import (
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
)

type CloudConnectionRepository interface {
	CreateCloudConnection(*model.CloudConnection) error
	ListCloudConnections() ([]model.CloudConnection, error)
	GetCloudConnection(uint) (*model.CloudConnection, error)
	UpdateCloudConnection(*model.CloudConnection) error
	DeleteCloudConnection(uint) error
	UpdateCloudConnectionValidation(uint, string, string, string, time.Time) error
}

var _ CloudConnectionRepository = (*store.Store)(nil)
