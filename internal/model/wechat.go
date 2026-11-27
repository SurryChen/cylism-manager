package model

import (
	"errors"
	"time"
)

var (
	ErrWeChatChannel           = errors.New("请选择本项目的微信小程序或小游戏渠道")
	ErrWeChatAppID             = errors.New("该 AppID 已绑定，或与已绑定的微信账号不一致")
	ErrWeChatConnectionChanged = errors.New("微信连接已变更，请重试")
)

// ProductWeChatConnection is scoped to an existing product distribution channel.
type ProductWeChatConnection struct {
	ChannelID          uint   `gorm:"primaryKey;autoIncrement:false" json:"channel_id"`
	ProductID          uint   `gorm:"index;not null" json:"product_id"`
	AppID              string `gorm:"size:18;uniqueIndex;not null" json:"app_id"`
	EncryptedAppSecret string `json:"-"`
}

type WeChatDailyMetric struct {
	ChannelID uint      `gorm:"primaryKey;autoIncrement:false" json:"-"`
	Day       string    `gorm:"primaryKey;size:10" json:"day"`
	Metric    string    `gorm:"primaryKey;size:32" json:"metric"`
	Value     *float64  `json:"value"`
	FetchedAt time.Time `json:"fetched_at"`
}
