package api

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cylism/cylism-manager/internal/crypto"
	"github.com/cylism/cylism-manager/internal/model"
	runtimechat "github.com/cylism/cylism-manager/internal/runtime/chat"
	"github.com/cylism/cylism-manager/internal/store"
	"github.com/gin-gonic/gin"
)

type fakeChatClient struct {
	streamBody   string
	sessions     []runtimechat.Session
	detail       *runtimechat.SessionDetail
	listErr      error
	readErr      error
	streamErr    error
	streamStatus int
	readOptions  runtimechat.SessionHistoryOptions
	renameTitle  string
	archived     bool
	deleted      bool
}

func (f *fakeChatClient) StreamChat(_ context.Context, _ string, _ string) (*http.Response, error) {
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	status := f.streamStatus
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(f.streamBody)), Header: http.Header{}}, nil
}

func (f *fakeChatClient) ListSessions(_ context.Context, _ bool) ([]runtimechat.Session, error) {
	return f.sessions, f.listErr
}

func (f *fakeChatClient) ReadSession(_ context.Context, _ string, options runtimechat.SessionHistoryOptions) (*runtimechat.SessionDetail, error) {
	f.readOptions = options
	return f.detail, f.readErr
}

func (f *fakeChatClient) RenameSession(_ context.Context, _ string, title string) (*runtimechat.Session, error) {
	f.renameTitle = title
	return &runtimechat.Session{ID: "abc", Title: title}, nil
}

func (f *fakeChatClient) ArchiveSession(_ context.Context, _ string, archived bool) error {
	f.archived = archived
	return nil
}

func (f *fakeChatClient) ExportSession(_ context.Context, _ string) (*runtimechat.SessionExport, error) {
	return &runtimechat.SessionExport{ID: "abc", Snapshot: map[string]any{"messages": []any{}}}, nil
}

func (f *fakeChatClient) DeleteSession(_ context.Context, _ string) error {
	f.deleted = true
	return nil
}

func setupChatRouter(t *testing.T, client runtimechat.ChatClient, seenKey *string) (*gin.Engine, *store.Store, []byte) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	encKey := []byte("01234567890123456789012345678901")
	handler := NewRuntimeHandler(s, encKey, nil, nil)
	handler.newChatClient = func(instance *model.RuntimeInstance, apiKey string) (runtimechat.ChatClient, error) {
		if seenKey != nil {
			*seenKey = apiKey
		}
		return client, nil
	}
	router := gin.New()
	group := router.Group("/api/runtimes")
	group.POST("/:id/chat", handler.Chat)
	group.GET("/:id/chat/sessions", handler.ChatSessions)
	group.GET("/:id/chat/sessions/:sid/messages", handler.ChatSessionMessages)
	group.PATCH("/:id/chat/sessions/:sid", handler.RenameChatSession)
	group.POST("/:id/chat/sessions/:sid/archive", handler.ArchiveChatSession)
	group.POST("/:id/chat/sessions/:sid/restore", handler.RestoreChatSession)
	group.GET("/:id/chat/sessions/:sid/export", handler.ExportChatSession)
	group.DELETE("/:id/chat/sessions/:sid", handler.DeleteChatSession)
	return router, s, encKey
}

func createChatRuntime(t *testing.T, s *store.Store, encKey []byte) uint {
	t.Helper()
	encrypted, err := crypto.Encrypt(encKey, "runtime-secret")
	if err != nil {
		t.Fatalf("encrypt runtime key: %v", err)
	}
	instance := &model.RuntimeInstance{
		Name: "nanobot-main", RuntimeType: "nanobot", DeploymentMode: "managed",
		Image: "example/nanobot:latest", Namespace: "cylism-assistant", PVCName: "nanobot-main-data",
		Storage: "10Gi", ModelName: "gpt-test", Status: "ready", EncryptedRuntimeAPIKey: encrypted,
	}
	if err := s.CreateRuntime(instance); err != nil {
		t.Fatalf("create runtime: %v", err)
	}
	return instance.ID
}

func TestChatStreamsSSEWithRuntimeKey(t *testing.T) {
	client := &fakeChatClient{streamBody: "data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\ndata: [DONE]\n\n"}
	router, s, encKey := setupChatRouter(t, client, nil)
	id := createChatRuntime(t, s, encKey)

	response := serve(router, newJSONRequest(http.MethodPost, "/api/runtimes/"+itoa(id)+"/chat", gin.H{"message": "你好"}))
	if response.Code != http.StatusOK {
		t.Fatalf("chat status = %d: %s", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/event-stream" {
		t.Fatalf("unexpected content type: %s", contentType)
	}
	body := response.Body.String()
	if !strings.Contains(body, `"type":"delta"`) || !strings.Contains(body, `"content":"好"`) || !strings.Contains(body, `"type":"done"`) {
		t.Fatalf("unexpected SSE body: %s", body)
	}
}

func TestChatRejectsEmptyMessage(t *testing.T) {
	router, s, encKey := setupChatRouter(t, &fakeChatClient{}, nil)
	id := createChatRuntime(t, s, encKey)
	response := serve(router, newJSONRequest(http.MethodPost, "/api/runtimes/"+itoa(id)+"/chat", gin.H{"message": "  "}))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected validation failure, got %d: %s", response.Code, response.Body.String())
	}
}

func TestChatUsesDecryptedRuntimeKey(t *testing.T) {
	var seenKey string
	router, s, encKey := setupChatRouter(t, &fakeChatClient{streamBody: "data: [DONE]\n\n"}, &seenKey)
	id := createChatRuntime(t, s, encKey)
	response := serve(router, newJSONRequest(http.MethodPost, "/api/runtimes/"+itoa(id)+"/chat", gin.H{"message": "hi"}))
	if response.Code != http.StatusOK {
		t.Fatalf("chat status = %d: %s", response.Code, response.Body.String())
	}
	if seenKey != "runtime-secret" {
		t.Fatalf("expected decrypted runtime key, got %q", seenKey)
	}
}

func TestChatSessionsListsRuntimeSessions(t *testing.T) {
	client := &fakeChatClient{sessions: []runtimechat.Session{{ID: "abc", Title: "Hello", UpdatedAt: "2026-01-01T00:00:00Z"}}}
	router, s, encKey := setupChatRouter(t, client, nil)
	id := createChatRuntime(t, s, encKey)
	response := serve(router, newJSONRequest(http.MethodGet, "/api/runtimes/"+itoa(id)+"/chat/sessions", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("sessions status = %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"id":"abc"`) {
		t.Fatalf("unexpected sessions body: %s", response.Body.String())
	}
}

func TestChatSessionMessagesReturnsHistory(t *testing.T) {
	client := &fakeChatClient{detail: &runtimechat.SessionDetail{ID: "abc", Title: "Hello", Messages: []runtimechat.Message{{Role: "user", Content: "hi", CreatedAt: "2026-01-01T00:00:00Z"}}}}
	router, s, encKey := setupChatRouter(t, client, nil)
	id := createChatRuntime(t, s, encKey)
	response := serve(router, newJSONRequest(http.MethodGet, "/api/runtimes/"+itoa(id)+"/chat/sessions/abc/messages", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("messages status = %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"content":"hi"`) {
		t.Fatalf("unexpected messages body: %s", response.Body.String())
	}
	if client.readOptions != (runtimechat.SessionHistoryOptions{Limit: 50}) {
		t.Fatalf("unexpected default options: %#v", client.readOptions)
	}
}

func TestChatSessionMessagesPassesPageOptions(t *testing.T) {
	client := &fakeChatClient{detail: &runtimechat.SessionDetail{ID: "abc"}}
	router, s, encKey := setupChatRouter(t, client, nil)
	id := createChatRuntime(t, s, encKey)
	response := serve(router, newJSONRequest(http.MethodGet, "/api/runtimes/"+itoa(id)+"/chat/sessions/abc/messages?limit=20&before=30", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("messages status = %d: %s", response.Code, response.Body.String())
	}
	if client.readOptions != (runtimechat.SessionHistoryOptions{Limit: 20, Before: "30"}) {
		t.Fatalf("unexpected options: %#v", client.readOptions)
	}
}

func TestChatSessionMessagesRejectsInvalidLimit(t *testing.T) {
	router, s, encKey := setupChatRouter(t, &fakeChatClient{}, nil)
	id := createChatRuntime(t, s, encKey)
	response := serve(router, newJSONRequest(http.MethodGet, "/api/runtimes/"+itoa(id)+"/chat/sessions/abc/messages?limit=0", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected validation failure, got %d: %s", response.Code, response.Body.String())
	}
}

func TestChatSessionMessagesMissingSessionReturns404(t *testing.T) {
	router, s, encKey := setupChatRouter(t, &fakeChatClient{}, nil)
	id := createChatRuntime(t, s, encKey)
	response := serve(router, newJSONRequest(http.MethodGet, "/api/runtimes/"+itoa(id)+"/chat/sessions/nope/messages", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", response.Code, response.Body.String())
	}
}

func TestChatSessionLifecycleProxiesNativeRuntimeActions(t *testing.T) {
	client := &fakeChatClient{}
	router, s, encKey := setupChatRouter(t, client, nil)
	id := createChatRuntime(t, s, encKey)

	rename := serve(router, newJSONRequest(http.MethodPatch, "/api/runtimes/"+itoa(id)+"/chat/sessions/abc", gin.H{"title": "排查镜像问题"}))
	if rename.Code != http.StatusOK || client.renameTitle != "排查镜像问题" {
		t.Fatalf("rename failed: status=%d title=%q body=%s", rename.Code, client.renameTitle, rename.Body.String())
	}
	archive := serve(router, newJSONRequest(http.MethodPost, "/api/runtimes/"+itoa(id)+"/chat/sessions/abc/archive", nil))
	if archive.Code != http.StatusOK || !client.archived {
		t.Fatalf("archive failed: status=%d archived=%v", archive.Code, client.archived)
	}
	restore := serve(router, newJSONRequest(http.MethodPost, "/api/runtimes/"+itoa(id)+"/chat/sessions/abc/restore", nil))
	if restore.Code != http.StatusOK || client.archived {
		t.Fatalf("restore failed: status=%d archived=%v", restore.Code, client.archived)
	}
	export := serve(router, newJSONRequest(http.MethodGet, "/api/runtimes/"+itoa(id)+"/chat/sessions/abc/export", nil))
	if export.Code != http.StatusOK || !strings.Contains(export.Body.String(), `"snapshot"`) {
		t.Fatalf("export failed: status=%d body=%s", export.Code, export.Body.String())
	}
	deleted := serve(router, newJSONRequest(http.MethodDelete, "/api/runtimes/"+itoa(id)+"/chat/sessions/abc", nil))
	if deleted.Code != http.StatusOK || !client.deleted {
		t.Fatalf("delete failed: status=%d deleted=%v", deleted.Code, client.deleted)
	}
}

func TestChatSessionLifecycleRejectsInternalKeyAndInvalidTitle(t *testing.T) {
	router, s, encKey := setupChatRouter(t, &fakeChatClient{}, nil)
	id := createChatRuntime(t, s, encKey)
	internal := serve(router, newJSONRequest(http.MethodDelete, "/api/runtimes/"+itoa(id)+"/chat/sessions/api:abc", nil))
	if internal.Code != http.StatusBadRequest {
		t.Fatalf("internal key status = %d: %s", internal.Code, internal.Body.String())
	}
	empty := serve(router, newJSONRequest(http.MethodPatch, "/api/runtimes/"+itoa(id)+"/chat/sessions/abc", gin.H{"title": " "}))
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("empty title status = %d: %s", empty.Code, empty.Body.String())
	}
}
