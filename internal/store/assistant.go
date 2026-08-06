package store

import "github.com/cylism/cylism-manager/internal/model"

func (s *Store) CreateAssistantProvider(provider *model.AssistantProvider) error {
	return s.db.Create(provider).Error
}

func (s *Store) ListAssistantProviders() ([]model.AssistantProvider, error) {
	var providers []model.AssistantProvider
	return providers, s.db.Order("id asc").Find(&providers).Error
}

func (s *Store) GetAssistantProvider(id uint) (*model.AssistantProvider, error) {
	var provider model.AssistantProvider
	if err := s.db.First(&provider, id).Error; err != nil {
		return nil, err
	}
	return &provider, nil
}

func (s *Store) SaveAssistantProvider(provider *model.AssistantProvider) error {
	return s.db.Save(provider).Error
}

func (s *Store) DeleteAssistantProvider(id uint) error {
	return s.db.Delete(&model.AssistantProvider{}, id).Error
}

func (s *Store) CreateAssistantConversation(conversation *model.AssistantConversation) error {
	return s.db.Create(conversation).Error
}

func (s *Store) ListAssistantConversations(userID uint, limit int) ([]model.AssistantConversation, error) {
	var conversations []model.AssistantConversation
	query := s.db.Where("user_id = ?", userID).Order("updated_at desc")
	if limit > 0 {
		query = query.Limit(limit)
	}
	return conversations, query.Find(&conversations).Error
}

func (s *Store) GetAssistantConversation(id, userID uint) (*model.AssistantConversation, error) {
	var conversation model.AssistantConversation
	if err := s.db.Where("id = ? AND user_id = ?", id, userID).First(&conversation).Error; err != nil {
		return nil, err
	}
	return &conversation, nil
}

func (s *Store) CreateAssistantMessage(message *model.AssistantMessage) error {
	return s.db.Create(message).Error
}

func (s *Store) ListAssistantMessages(conversationID uint) ([]model.AssistantMessage, error) {
	var messages []model.AssistantMessage
	return messages, s.db.Where("conversation_id = ?", conversationID).Order("id asc").Find(&messages).Error
}
