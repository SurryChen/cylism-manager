package wechat

import (
	"context"
	"crypto/sha256"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	"golang.org/x/sync/errgroup"
)

type metric struct {
	name, gameID string
}

var gameMetrics = []metric{
	{"active_users", "1000001:5"}, {"new_users", "1000011:3"}, {"visits", "1000001:6"},
	{"next_day_retention", "1000021:3"}, {"shares", "1000028:3"}, {"ad_revenue", "1000172:6"},
}

const dayFormat = "2006-01-02"

type Analytics struct {
	Samples      []model.WeChatDailyMetric `json:"samples"`
	SyncedAt     *time.Time                `json:"synced_at,omitempty"`
	From         string                    `json:"from"`
	Through      string                    `json:"through"`
	Error        string                    `json:"error,omitempty"`
	MetricErrors map[string]string         `json:"metric_errors,omitempty"`
}

func (s *Service) Analytics(ctx context.Context, owner, product, channel uint, days int, force bool) (Analytics, error) {
	item, err := s.repo.WeChatConnection(owner, product, channel)
	if err != nil {
		return Analytics{}, err
	}
	if item.EncryptedAppSecret == "" {
		return Analytics{}, ErrAnalyticsNotConfigured
	}
	platform, err := s.repo.WeChatChannelPlatform(item)
	if err != nil {
		return Analytics{}, err
	}
	return s.analytics(ctx, item, platform, days, force)
}

func (s *Service) analytics(ctx context.Context, item model.ProductWeChatConnection, platform string, days int, force bool) (Analytics, error) {
	today := time.Now().In(Beijing)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, Beijing)
	from := today.AddDate(0, 0, -days).Format(dayFormat)
	cache, err := s.repo.WeChatMetrics(item.ChannelID, from)
	if err != nil {
		return Analytics{}, err
	}
	key := strconv.FormatUint(uint64(item.ChannelID), 10) + ":" + strconv.Itoa(days) + ":" + item.EncryptedAppSecret
	wait := s.work.DoChan("analytics:"+key, func() (any, error) {
		workCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
		defer cancel()
		return s.fetchMissing(workCtx, item, platform, today, days, cache, force)
	})
	var fetchErr error
	var metricErrors map[string]string
	select {
	case <-ctx.Done():
		return Analytics{}, ctx.Err()
	case result := <-wait:
		fetchErr = result.Err
		metricErrors, _ = result.Val.(map[string]string)
	}
	samples, err := s.repo.WeChatMetrics(item.ChannelID, from)
	if err != nil {
		return Analytics{}, err
	}
	result := Analytics{Samples: samples, From: from, Through: today.AddDate(0, 0, -1).Format(dayFormat), MetricErrors: metricErrors}
	for _, sample := range samples {
		if result.SyncedAt == nil || sample.FetchedAt.After(*result.SyncedAt) {
			stamp := sample.FetchedAt
			result.SyncedAt = &stamp
		}
	}
	if fetchErr != nil {
		result.Error = fetchErr.Error()
	}
	return result, nil
}

func (s *Service) directToken(item model.ProductWeChatConnection) (string, func(context.Context) (string, int, error)) {
	digest := sha256.Sum256([]byte(item.EncryptedAppSecret))
	return "direct:" + item.AppID + ":" + string(digest[:]), func(ctx context.Context) (string, int, error) {
		secret, err := security.Decrypt(s.key, item.EncryptedAppSecret)
		if err != nil {
			return "", 0, errors.New("微信 AppSecret 无法解密")
		}
		var output struct {
			Token   string `json:"access_token"`
			Expires int    `json:"expires_in"`
		}
		err = s.client.call(ctx, http.MethodPost, "/cgi-bin/stable_token", "", map[string]any{
			"grant_type": "client_credential", "appid": item.AppID, "secret": secret, "force_refresh": false,
		}, &output)
		return output.Token, output.Expires, err
	}
}

func (s *Service) fetchMissing(ctx context.Context, item model.ProductWeChatConnection, platform string, today time.Time, days int, cache []model.WeChatDailyMetric, force bool) (map[string]string, error) {
	now := time.Now()
	known := make(map[string]model.WeChatDailyMetric, len(cache))
	for _, sample := range cache {
		known[sample.Day+"/"+sample.Metric] = sample
	}
	keep := 6 * time.Hour
	if force {
		keep = time.Minute
	}
	stale := func(day time.Time, name string) bool {
		sample, ok := known[day.Format(dayFormat)+"/"+name]
		if !ok {
			return true
		}
		return day.After(today.AddDate(0, 0, -9)) && now.Sub(sample.FetchedAt) > keep
	}
	metrics := gameMetrics
	if platform != model.ProductChannelMiniGame {
		metrics = []metric{{name: "active_users"}}
	}
	type fetchResult struct {
		samples []model.WeChatDailyMetric
		err     error
	}
	results := make([]fetchResult, len(metrics))
	key, load := s.directToken(item)
	var group errgroup.Group
	group.SetLimit(4)
	for i, m := range metrics {
		group.Go(func() error {
			result := &results[i]
			for offset := 1; offset <= days; offset++ {
				day := today.AddDate(0, 0, -offset)
				if !stale(day, m.name) || (m.name == "next_day_retention" && offset < 2) {
					continue
				}
				if ctx.Err() != nil {
					result.err = ctx.Err()
					break
				}
				if platform == model.ProductChannelMiniGame {
					sample, err := s.gameDaily(ctx, key, load, item.ChannelID, day, m)
					result.err = err
					if err == nil {
						result.samples = append(result.samples, sample)
					}
				} else {
					samples, err := s.miniDaily(ctx, key, load, item.ChannelID, day)
					result.err = err
					if err == nil {
						result.samples = append(result.samples, samples...)
					}
				}
				if result.err != nil {
					break
				}
			}
			return nil
		})
	}
	_ = group.Wait()
	var samples []model.WeChatDailyMetric
	metricErrors := make(map[string]string)
	for i, result := range results {
		for _, sample := range result.samples {
			if previous := known[sample.Day+"/"+sample.Metric]; sample.Value == nil {
				sample.Value = previous.Value
			}
			samples = append(samples, sample)
		}
		if result.err == nil {
			continue
		}
		message := result.err.Error()
		if ctx.Err() != nil {
			message = "本次查询超时，已保留取得的数据"
		}
		if platform != model.ProductChannelMiniGame {
			for _, name := range []string{"active_users", "new_users", "visits"} {
				metricErrors[name] = message
			}
		} else {
			metricErrors[metrics[i].name] = message
		}
	}
	return metricErrors, s.repo.SaveWeChatMetrics(item, samples, today.AddDate(0, 0, -90).Format(dayFormat))
}

func (s *Service) gameDaily(ctx context.Context, key string, load func(context.Context) (string, int, error), channel uint, day time.Time, m metric) (model.WeChatDailyMetric, error) {
	var output struct {
		Data []struct {
			Stamp int64    `json:"timestamp"`
			Value *float64 `json:"metric_value"`
		} `json:"data_list"`
	}
	queryDay := day
	// Retention becomes available on the following day. WeChat may emit
	// the observation timestamp on that day, while the chart uses cohort date.
	if m.name == "next_day_retention" {
		queryDay = day.AddDate(0, 0, 1)
	}
	start := queryDay.Unix()
	err := s.client.doWithToken(ctx, key, load, func(access string) error {
		return s.client.call(ctx, http.MethodPost, "/datacube/getgameanalysisdata", access, map[string]any{
			"metric": m.gameID, "granularity": 24, "start_time": start, "end_time": queryDay.AddDate(0, 0, 1).Unix(),
		}, &output)
	})
	sample := model.WeChatDailyMetric{ChannelID: channel, Day: day.Format(dayFormat), Metric: m.name, FetchedAt: time.Now().UTC()}
	for _, row := range output.Data {
		if row.Stamp == start {
			sample.Value = row.Value
			break
		}
	}
	return sample, err
}

func (s *Service) miniDaily(ctx context.Context, key string, load func(context.Context) (string, int, error), channel uint, day time.Time) ([]model.WeChatDailyMetric, error) {
	var output struct {
		List []struct {
			Date   string   `json:"ref_date"`
			Users  *float64 `json:"visit_uv"`
			New    *float64 `json:"visit_uv_new"`
			Visits *float64 `json:"visit_pv"`
		} `json:"list"`
	}
	date := day.Format("20060102")
	err := s.client.doWithToken(ctx, key, load, func(access string) error {
		return s.client.call(ctx, http.MethodPost, "/datacube/getweanalysisappiddailyvisittrend", access, map[string]string{
			"begin_date": date, "end_date": date,
		}, &output)
	})
	var active, newUsers, visits *float64
	for _, row := range output.List {
		if row.Date == date {
			active, newUsers, visits = row.Users, row.New, row.Visits
			break
		}
	}
	now := time.Now().UTC()
	return []model.WeChatDailyMetric{
		{ChannelID: channel, Day: day.Format(dayFormat), Metric: "active_users", FetchedAt: now, Value: active},
		{ChannelID: channel, Day: day.Format(dayFormat), Metric: "new_users", FetchedAt: now, Value: newUsers},
		{ChannelID: channel, Day: day.Format(dayFormat), Metric: "visits", FetchedAt: now, Value: visits},
	}, err
}

func (s *Service) PollAnalytics(ctx context.Context) {
	items, err := s.repo.WeChatAnalyticsTargets()
	if err != nil {
		log.Print("微信运营数据同步列表读取失败")
		return
	}
	for _, item := range items {
		if ctx.Err() != nil {
			return
		}
		platform, err := s.repo.WeChatChannelPlatform(item)
		if err != nil {
			continue
		}
		data, err := s.analytics(ctx, item, platform, 7, false)
		if err != nil || data.Error != "" || len(data.MetricErrors) != 0 {
			log.Printf("微信运营数据同步失败，渠道 %d", item.ChannelID)
		}
	}
}
