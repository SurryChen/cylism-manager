package repository

import (
	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
)

// UserRepository is the authentication persistence boundary.
type UserRepository interface {
	CreateUser(*model.User) error
	GetUserByUsername(string) (*model.User, error)
	CountUsers() (int64, error)
}

var _ UserRepository = (*store.Store)(nil)
