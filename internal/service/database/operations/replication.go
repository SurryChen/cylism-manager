package operations

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"gorm.io/gorm"
)

type replicationObservation struct {
	Channel        string `json:"channel"`
	SourceVerified bool   `json:"source_verified,omitempty"`
	IORunning      bool   `json:"io_running"`
	SQLRunning     bool   `json:"sql_running"`
	LagSeconds     *int64 `json:"lag_seconds"`
	IOErrno        int    `json:"io_errno"`
	SQLErrno       int    `json:"sql_errno"`
	ReadOnly       bool   `json:"read_only"`
	SuperReadOnly  bool   `json:"super_read_only"`
	Schemas        string `json:"schemas"`
	Tables         string `json:"tables"`
}

// Called in the same transaction that completes the read-only task. Discovery
// only records a channel whose source UUID was checked against the selected
// source. It never creates credentials, initializes data or changes replication.
func recordReplicationObservation(tx *gorm.DB, task *model.DatabaseOperation, result string) error {
	var observation replicationObservation
	if json.Unmarshal([]byte(result), &observation) != nil || len(observation.Channel) > 64 || strings.ContainsRune(observation.Channel, 0) {
		return fmt.Errorf("复制状态结果无效")
	}
	var relation model.DatabaseReplication
	err := tx.Where("target = ?", task.Target).First(&relation).Error
	if task.Kind == "replication_discover" {
		if !observation.SourceVerified {
			return fmt.Errorf("复制源身份尚未核对")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			relation = model.DatabaseReplication{Source: task.Source, Target: task.Target, Channel: observation.Channel, TaskID: task.ID}
		} else if err != nil {
			return err
		} else if relation.Source != task.Source || relation.Channel != observation.Channel {
			return fmt.Errorf("已有复制关系与发现结果冲突")
		}
	} else if err != nil {
		return err
	} else if relation.Source != task.Source || relation.Channel != observation.Channel {
		return fmt.Errorf("复制通道记录已变化")
	}
	now := time.Now()
	relation.Status = "error"
	if observation.IORunning && observation.SQLRunning && observation.IOErrno == 0 && observation.SQLErrno == 0 {
		relation.Status = "running"
	}
	relation.LastCheckedAt = &now
	relation.IORunning, relation.SQLRunning = observation.IORunning, observation.SQLRunning
	relation.LagSeconds, relation.ReadOnly = observation.LagSeconds, observation.ReadOnly
	raw, err := json.Marshal(observation)
	if err != nil {
		return err
	}
	relation.Observation = string(raw)
	return tx.Save(&relation).Error
}
