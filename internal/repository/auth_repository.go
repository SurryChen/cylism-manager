package repository

import (
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
)

// UserRepository is the authentication persistence boundary.
type UserRepository interface {
	CreateUser(*model.User) error
	GetUserByUsername(string) (*model.User, error)
	GetUserByID(uint) (*model.User, error)
	CountUsers() (int64, error)
}

type TemporaryLoginTokenRepository interface {
	CreateTemporaryLoginToken(*model.TemporaryLoginToken) error
	ListTemporaryLoginTokens(uint) ([]model.TemporaryLoginToken, error)
	FindActiveTemporaryLoginToken(string, time.Time) (*model.TemporaryLoginToken, error)
	MarkTemporaryLoginTokenUsed(string, time.Time) error
	RevokeTemporaryLoginToken(uint, uint, time.Time) error
}

var _ UserRepository = (*store.Store)(nil)
var _ TemporaryLoginTokenRepository = (*store.Store)(nil)
