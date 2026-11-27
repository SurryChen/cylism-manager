package wechat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/store"
)

func wechatFixture(t *testing.T) (*Service, *store.Store, model.Product) {
	t.Helper()
	database, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	product := model.Product{OwnerID: 9, Name: "同频词遇", Stage: "developing", Channels: []model.ProductChannel{{Platform: model.ProductChannelMiniGame}}}
	if err = database.SaveProduct(&product, nil, nil); err != nil {
		t.Fatal(err)
	}
	service := New(database, make([]byte, 32))
	if err = service.Connect(product.OwnerID, product.ID, product.Channels[0].ID, "wx1234567890abcdef", "game-secret"); err != nil {
		t.Fatal(err)
	}
	return service, database, product
}

func TestGameAnalyticsKeepsAvailableDataWhenMetricFails(t *testing.T) {
	service, _, product := wechatFixture(t)
	var tokenRequests atomic.Int32
	var metricRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cgi-bin/stable_token" {
			tokenRequests.Add(1)
			var input map[string]any
			if json.NewDecoder(r.Body).Decode(&input) != nil || input["secret"] != "game-secret" || input["force_refresh"] != false {
				t.Error("invalid stable token request")
			}
			fmt.Fprint(w, `{"access_token":"game-access","expires_in":7200}`)
			return
		}
		metricRequests.Add(1)
		var input struct {
			Metric string `json:"metric"`
			Start  int64  `json:"start_time"`
			End    int64  `json:"end_time"`
			Grain  int    `json:"granularity"`
		}
		if r.URL.Path != "/datacube/getgameanalysisdata" || r.URL.Query().Get("access_token") != "game-access" ||
			json.NewDecoder(r.Body).Decode(&input) != nil || input.End-input.Start != 86400 || input.Grain != 24 {
			t.Error("invalid game daily request")
		}
		if input.Metric == "1000172:6" {
			fmt.Fprint(w, `{"errcode":48001}`)
			return
		}
		fmt.Fprintf(w, `{"data_list":[{"timestamp":%d,"metric_value":0}]}`, input.Start)
	}))
	defer server.Close()
	service.client.base = server.URL
	data, err := service.Analytics(context.Background(), product.OwnerID, product.ID, product.Channels[0].ID, 2, false)
	if err != nil || data.Error != "" || data.MetricErrors["ad_revenue"] == "" || len(data.Samples) != 9 || tokenRequests.Load() != 1 {
		t.Fatalf("partial analytics: %#v, %v; token calls: %d", data, err, tokenRequests.Load())
	}
	for _, sample := range data.Samples {
		if sample.Value == nil || *sample.Value != 0 {
			t.Fatal("observed zero must be stored as a value")
		}
	}
	previousCalls := metricRequests.Load()
	if _, err := service.Analytics(context.Background(), product.OwnerID, product.ID, product.Channels[0].ID, 2, false); err != nil {
		t.Fatal(err)
	}
	if metricRequests.Load()-previousCalls != 1 {
		t.Fatal("cached metrics were unnecessarily requested again")
	}
}
