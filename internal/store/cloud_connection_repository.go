package store

import (
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

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
func (s *Store) UpdateCloudConnectionValidation(id uint, dnsStatus, storageStatus, detail string, validatedAt time.Time) error {
	return s.db.Model(&model.CloudConnection{}).Where("id = ?", id).Updates(map[string]any{"dns_status": dnsStatus, "object_storage_status": storageStatus, "last_validation_error": detail, "last_validation_at": validatedAt}).Error
}
