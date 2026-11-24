package model

import "time"

// ServerSpeedTest stores measured results without credentials or raw output.
type ServerSpeedTest struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	OwnerID       uint      `gorm:"index:idx_speed_owner_created" json:"-"`
	SourceID      uint      `json:"source_id"`
	TargetID      uint      `json:"target_id"`
	TargetAddress string    `json:"target_address"`
	Address       string    `json:"address"`
	Protocol      string    `json:"protocol"`
	Direction     string    `json:"direction"`
	SenderMbps    float64   `json:"sender_mbps"`
	ReceiverMbps  float64   `json:"receiver_mbps"`
	Retransmits   int       `json:"retransmits"`
	LostPackets   int       `json:"lost_packets"`
	TotalPackets  int       `json:"total_packets"`
	LossPercent   float64   `json:"loss_percent"`
	CreatedAt     time.Time `gorm:"index:idx_speed_owner_created" json:"measuredAt"`
}
