package wechat

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
	service := New(database, make([]byte, 32), "https://manager.example", ComponentConfig{
		AppID: "wx0000000000000000", AppSecret: "component-secret", EventToken: "signature-token",
		EncodingAESKey: base64.RawStdEncoding.EncodeToString(make([]byte, 32)),
	})
	if err = service.Connect(product.OwnerID, product.ID, product.Channels[0].ID, "wx1234567890abcdef", "game-secret"); err != nil {
		t.Fatal(err)
	}
	return service, database, product
}

func signedTicket(t *testing.T, cfg ComponentConfig) ([]byte, string, string) {
	t.Helper()
	now := time.Now().Unix()
	message := fmt.Sprintf("<xml><AppId>%s</AppId><CreateTime>%d</CreateTime><InfoType>component_verify_ticket</InfoType><ComponentVerifyTicket>event-ticket</ComponentVerifyTicket></xml>", cfg.AppID, now)
	plain := make([]byte, 20)
	binary.BigEndian.PutUint32(plain[16:], uint32(len(message)))
	plain = append(plain, message...)
	plain = append(plain, cfg.AppID...)
	padding := 32 - len(plain)%32
	plain = append(plain, bytes.Repeat([]byte{byte(padding)}, padding)...)
	key, _ := base64.RawStdEncoding.DecodeString(cfg.EncodingAESKey)
	block, _ := aes.NewCipher(key)
	cipher.NewCBCEncrypter(block, key[:aes.BlockSize]).CryptBlocks(plain, plain)
	encrypted := base64.StdEncoding.EncodeToString(plain)
	timestamp := strconv.FormatInt(now, 10)
	parts := []string{cfg.EventToken, timestamp, "nonce", encrypted}
	sort.Strings(parts)
	signature := sha1.Sum([]byte(strings.Join(parts, "")))
	return []byte("<xml><Encrypt>" + encrypted + "</Encrypt></xml>"), timestamp, hex.EncodeToString(signature[:])
}

func TestWeChatAuthorizationAndReleaseSync(t *testing.T) {
	service, database, product := wechatFixture(t)
	body, timestamp, signature := signedTicket(t, service.component)
	if err := service.ReceiveEvent(body, timestamp, "changed", signature); err != ErrInvalidEvent {
		t.Fatalf("changed nonce accepted: %v", err)
	}
	if err := service.ReceiveEvent(body, timestamp, "nonce", signature); err != nil {
		t.Fatal(err)
	}
	published := time.Date(2026, 11, 27, 2, 43, 28, 0, time.UTC)
	var auditUnavailable atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		query := r.URL.Query()
		var input map[string]string
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
			}
		}
		switch r.URL.Path {
		case "/cgi-bin/component/api_component_token":
			if input["component_verify_ticket"] != "event-ticket" || input["component_appsecret"] != "component-secret" {
				t.Error("component token request does not contain the decrypted ticket and configured secret")
			}
			fmt.Fprint(w, `{"component_access_token":"component-access","expires_in":7200}`)
		case "/cgi-bin/component/api_create_preauthcode", "/cgi-bin/component/api_query_auth":
			if query.Get("component_access_token") != "component-access" || query.Has("access_token") {
				t.Error("component access token must use the official query parameter")
			}
			if r.URL.Path == "/cgi-bin/component/api_create_preauthcode" {
				fmt.Fprint(w, `{"pre_auth_code":"pre-code"}`)
				return
			}
			if input["authorization_code"] != "authorization-code" {
				t.Error("wrong authorization code")
			}
			fmt.Fprint(w, `{"authorization_info":{"authorizer_appid":"wx1234567890abcdef","authorizer_access_token":"grant-access","authorizer_refresh_token":"grant-refresh","expires_in":7200,"func_info":[{"funcscope_category":{"id":18}}]}}`)
		case "/wxa/getversioninfo":
			if r.Method != http.MethodPost || query.Get("access_token") != "grant-access" || len(input) != 0 {
				t.Error("wrong version request")
			}
			fmt.Fprintf(w, `{"exp_info":{"exp_version":"0.5.0"},"release_info":{"release_version":"0.4.0","release_time":%d,"release_desc":"正式发行"}}`, published.Unix())
		case "/wxa/get_latest_auditstatus":
			if r.Method != http.MethodGet || query.Get("access_token") != "grant-access" {
				t.Error("wrong audit request")
			}
			if auditUnavailable.Load() {
				fmt.Fprint(w, `{"errcode":48001}`)
			} else {
				fmt.Fprint(w, `{"status":0}`)
			}
		default:
			t.Errorf("unexpected API: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	service.client.base = server.URL
	channel := product.Channels[0].ID
	target, err := service.StartAuthorization(context.Background(), product.OwnerID, product.ID, channel)
	if err != nil {
		t.Fatal(err)
	}
	login, err := url.Parse(target)
	if err != nil || login.Hostname() != "mp.weixin.qq.com" || login.Query().Get("biz_appid") != "wx1234567890abcdef" {
		t.Fatalf("authorization link: %s, %v", target, err)
	}
	callback, err := url.Parse(login.Query().Get("redirect_uri"))
	if err != nil || callback.Hostname() != "manager.example" || callback.Path != AuthorizationPath {
		t.Fatalf("authorization callback: %v, %v", callback, err)
	}
	state := callback.Query().Get("state")
	item, err := service.FinishAuthorization(context.Background(), state, "authorization-code")
	if err != nil || item.ProductID != product.ID || item.ChannelID != channel {
		t.Fatalf("grant target: %d/%d, %v", item.ProductID, item.ChannelID, err)
	}
	if _, err := service.FinishAuthorization(context.Background(), state, "authorization-code"); err == nil {
		t.Fatal("authorization can be replayed")
	}
	if err := service.SyncRelease(context.Background(), product.OwnerID, product.ID, channel); err != nil {
		t.Fatal(err)
	}
	items, total, err := database.ListProductDistributions(product.OwnerID, product.ID, model.ProductLifecycleFilter{Limit: 10})
	if err != nil || total != 1 || items[0].Status != "published" || items[0].PlatformVersion != "0.4.0" {
		t.Fatalf("release sync: %#v, %d, %v", items, total, err)
	}
	settings, err := service.Settings(product.OwnerID, product.ID)
	if err != nil || len(settings.Connections) != 1 || settings.Connections[0].TrialVersion != "0.5.0" || settings.Connections[0].AuditStatus == nil {
		t.Fatalf("release settings: %#v, %v", settings, err)
	}
	auditUnavailable.Store(true)
	if err := service.SyncRelease(context.Background(), product.OwnerID, product.ID, channel); err != nil {
		t.Fatalf("audit failure blocked release sync: %v", err)
	}
	settings, err = service.Settings(product.OwnerID, product.ID)
	if err != nil || len(settings.Connections) != 1 || settings.Connections[0].AuditStatus != nil || settings.Connections[0].AuditError == "" {
		t.Fatalf("stale audit status or missing query error: %#v, %v", settings, err)
	}
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
