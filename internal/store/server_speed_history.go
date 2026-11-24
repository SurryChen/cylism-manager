package store

import "github.com/cylism/cylism-manager/internal/model"

func (s *Store) SaveServerSpeedTest(result *model.ServerSpeedTest) error {
	return s.db.Create(result).Error
}

func (s *Store) ListServerSpeedTests(ownerID uint, limit, offset int) ([]model.ServerSpeedTest, int64, error) {
	query := s.db.Model(&model.ServerSpeedTest{}).Where("owner_id = ?", ownerID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := []model.ServerSpeedTest{}
	err := query.Order("created_at desc, id desc").Limit(limit).Offset(offset).Find(&items).Error
	return items, total, err
}
