package store

import (
	"errors"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/runtime"
	"gorm.io/gorm"
)

func (s *Store) CreateRuntime(runtime *model.RuntimeInstance) error {
	return s.db.Create(runtime).Error
}

func (s *Store) GetRuntime(id uint) (*model.RuntimeInstance, error) {
	var runtime model.RuntimeInstance
	err := s.db.First(&runtime, id).Error
	return &runtime, err
}

func (s *Store) ListRuntimes() ([]model.RuntimeInstance, error) {
	var runtimes []model.RuntimeInstance
	err := s.db.Order("created_at desc").Find(&runtimes).Error
	return runtimes, err
}

func (s *Store) UpdateRuntime(runtime *model.RuntimeInstance) error {
	return s.db.Save(runtime).Error
}

func (s *Store) UpdateRuntimeHealth(id uint, status, detail string, checkedAt time.Time) error {
	return s.db.Model(&model.RuntimeInstance{}).Where("id = ?", id).Updates(map[string]interface{}{
		"health_status":  status,
		"health_detail":  detail,
		"last_health_at": checkedAt,
	}).Error
}

func (s *Store) DeleteRuntime(id uint) error {
	return s.db.Delete(&model.RuntimeInstance{}, id).Error
}

func (s *Store) ListAgentCapabilityGrants(runtimeID uint) ([]model.AgentCapabilityGrant, error) {
	var grants []model.AgentCapabilityGrant
	err := s.db.Where("runtime_id = ?", runtimeID).Order("capability asc, namespace asc").Find(&grants).Error
	return grants, err
}

func (s *Store) ReplaceAgentCapabilityGrants(runtimeID uint, grants []model.AgentCapabilityGrant) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("runtime_id = ?", runtimeID).Delete(&model.AgentCapabilityGrant{}).Error; err != nil {
			return err
		}
		for index := range grants {
			grant := grants[index]
			if grant.RuntimeID != runtimeID || !validAgentCapabilityGrant(grant) {
				return errors.New("invalid agent capability grant")
			}
			if err := tx.Create(&grant).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func validAgentCapabilityGrant(grant model.AgentCapabilityGrant) bool {
	if !model.ValidAgentCapability(grant.Capability) || (grant.Namespace != "*" && !runtime.NamespaceValid(grant.Namespace)) {
		return false
	}
	switch grant.Capability {
	case model.AgentCapabilityClusterRead, model.AgentCapabilityRegistryRead, model.AgentCapabilityRegistryVerify, model.AgentCapabilityRegistryPullCheck, model.AgentCapabilityDNSRead, model.AgentCapabilityRegistryProxyDiagnose, model.AgentCapabilityAlertRead, model.AgentCapabilityMonitoringRead, model.AgentCapabilityMaintenanceInspect, model.AgentCapabilityMaintenanceCleanup:
		return grant.Namespace == "*"
	default:
		return true
	}
}

func (s *Store) HasAgentCapability(runtimeID uint, capability, namespace string) (bool, error) {
	if !model.ValidAgentCapability(capability) {
		return false, nil
	}
	query := s.db.Model(&model.AgentCapabilityGrant{}).Where("runtime_id = ? AND capability = ? AND enabled = ?", runtimeID, capability, true)
	if namespace == "" {
		query = query.Where("namespace = ?", "*")
	} else {
		query = query.Where("namespace IN ?", []string{"*", namespace})
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// CreateAgentOperation creates one immutable Agent operation per Runtime and
// request ID. Retried CLI requests receive the original operation instead of
// creating a second approval or mutation.
func (s *Store) CreateAgentOperation(operation *model.AgentOperation) (*model.AgentOperation, bool, error) {
	if operation == nil || operation.RuntimeID == 0 || operation.RequestID == "" || operation.OperationID == "" || operation.Capability == "" || operation.Parameters == "" || operation.ParametersHash == "" || operation.Status == "" || operation.ExpiresAt.IsZero() {
		return nil, false, errors.New("invalid agent operation")
	}
	created := false
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing model.AgentOperation
		err := tx.Where("runtime_id = ? AND request_id = ?", operation.RuntimeID, operation.RequestID).First(&existing).Error
		if err == nil {
			*operation = existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Create(operation).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return operation, created, nil
}

func (s *Store) GetAgentOperation(operationID string) (*model.AgentOperation, error) {
	var operation model.AgentOperation
	err := s.db.Where("operation_id = ?", operationID).First(&operation).Error
	return &operation, err
}

func (s *Store) ListAgentOperations(runtimeID uint, limit int, status, sessionID string) ([]model.AgentOperation, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	query := s.db.Order("created_at desc").Limit(limit)
	if runtimeID > 0 {
		query = query.Where("runtime_id = ?", runtimeID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if sessionID != "" {
		query = query.Where("chat_session_id = ?", sessionID)
	}
	var operations []model.AgentOperation
	err := query.Find(&operations).Error
	return operations, err
}

// UpdateAgentOperationStatus only transitions non-terminal operations. This
// prevents an approval retry from executing or rewriting an already finished
// operation.
func (s *Store) UpdateAgentOperationStatus(operationID, fromStatus, toStatus, errorSummary string, approvedBy *uint, completedAt *time.Time) (bool, error) {
	updates := map[string]interface{}{"status": toStatus, "error_summary": errorSummary}
	if approvedBy != nil {
		updates["approved_by"] = *approvedBy
		updates["approved_at"] = time.Now()
	}
	if completedAt != nil {
		updates["completed_at"] = *completedAt
	}
	result := s.db.Model(&model.AgentOperation{}).Where("operation_id = ? AND status = ?", operationID, fromStatus).Updates(updates)
	return result.RowsAffected == 1, result.Error
}
