package store

import "github.com/cylism/cylism-manager/internal/model"

func (s *Store) CreateCloudConnection(connection *model.CloudConnection) error {
	return s.db.Create(connection).Error
}
func (s *Store) ListCloudConnections() ([]model.CloudConnection, error) {
	var connections []model.CloudConnection
	err := s.db.Order("created_at desc").Find(&connections).Error
	for index := range connections {
		connections[index].CredentialConfigured = connections[index].EncryptedCredentials != ""
	}
	return connections, err
}
func (s *Store) GetCloudConnection(id uint) (*model.CloudConnection, error) {
	var connection model.CloudConnection
	err := s.db.First(&connection, id).Error
	if err == nil {
		connection.CredentialConfigured = connection.EncryptedCredentials != ""
	}
	return &connection, err
}
func (s *Store) UpdateCloudConnection(connection *model.CloudConnection) error {
	return s.db.Save(connection).Error
}
func (s *Store) DeleteCloudConnection(id uint) error {
	return s.db.Delete(&model.CloudConnection{}, id).Error
}

func (s *Store) ListCloudConnectionsPage(limit, offset int) ([]model.CloudConnection, int64, error) {
	query := s.db.Model(&model.CloudConnection{})
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := []model.CloudConnection{}
	err := query.Order("created_at desc, id desc").Limit(limit).Offset(offset).Find(&items).Error
	for index := range items {
		items[index].CredentialConfigured = items[index].EncryptedCredentials != ""
	}
	return items, total, err
}
