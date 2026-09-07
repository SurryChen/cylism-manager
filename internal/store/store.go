package store

import (
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Store 数据存储层
type Store struct {
	db *gorm.DB
}

// New 创建新的 Store 实例，自动迁移所有模型
func New(dsn string) (*Store, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	if err := migrateSchema(db); err != nil {
		return nil, err
	}
	if err := cleanupLegacySchema(db); err != nil {
		return nil, err
	}

	store := &Store{db: db}
	if err := store.backfillData(); err != nil {
		return nil, err
	}
	return store, nil
}

// DB 返回底层 GORM DB 实例（供中间件等使用）
func (s *Store) DB() *gorm.DB {
	return s.db
}
