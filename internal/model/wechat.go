package model

import (
	"errors"
	"time"
)

var (
	ErrWeChatChannel       = errors.New("请选择本项目的微信小程序或小游戏渠道")
	ErrWeChatAppID         = errors.New("该 AppID 已绑定，或与已绑定的微信账号不一致")
	ErrWeChatAuthorization = errors.New("微信版本授权已失效，请重新连接")
)

// ProductWeChatConnection is scoped to an existing product distribution channel.
type ProductWeChatConnection struct {
	ChannelID             uint       `gorm:"primaryKey;autoIncrement:false" json:"channel_id"`
	ProductID             uint       `gorm:"index;not null" json:"product_id"`
	AppID                 string     `gorm:"size:18;uniqueIndex;not null" json:"app_id"`
	EncryptedAppSecret    string     `json:"-"`
	AuthorizedAt          int64      `json:"-"`
	ComponentAppID        string     `json:"-"`
	EncryptedRefreshToken string     `json:"-"`
	AuthStateHash         string     `gorm:"index" json:"-"`
	AuthExpiresAt         *time.Time `json:"-"`
	ReleaseCheckedAt      *time.Time `json:"release_checked_at,omitempty"`
	ReleaseError          string     `json:"release_error,omitempty"`
	OnlineVersion         string     `json:"online_version,omitempty"`
	OnlineAt              *time.Time `json:"online_at,omitempty"`
	TrialVersion          string     `json:"trial_version,omitempty"`
	AuditStatus           *int       `json:"audit_status,omitempty"`
	AuditReason           string     `json:"audit_reason,omitempty"`
	AuditError            string     `json:"audit_error,omitempty"`
	LastImportedAt        *time.Time `json:"-"`
}

type WeChatComponentTicket struct {
	AppID           string `gorm:"primaryKey;size:18"`
	EncryptedTicket string
	IssuedAt        int64
}

type WeChatDailyMetric struct {
	ChannelID uint      `gorm:"primaryKey;autoIncrement:false" json:"-"`
	Day       string    `gorm:"primaryKey;size:10" json:"day"`
	Metric    string    `gorm:"primaryKey;size:32" json:"metric"`
	Value     *float64  `json:"value"`
	FetchedAt time.Time `json:"fetched_at"`
}

type WeChatReleaseSnapshot struct {
	OnlineVersion string
	OnlineAt      *time.Time
	OnlineNotes   string
	TrialVersion  string
	AuditStatus   *int
	AuditReason   string
	AuditError    string
}
