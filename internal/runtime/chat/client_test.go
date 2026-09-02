package chat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamChatSendsAuthAndSession(t *testing.T) {
	var received map[string]any
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authHeader = request.Header.Get("Authorization")
		_ = json.NewDecoder(request.Body).Decode(&received)
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()

	client := NewRuntimeChatClient(server.URL+"/v1/chat/completions", server.URL, "gpt-test", "secret")
	response, err := client.StreamChat(context.Background(), "abc", "你好")
	if err != nil {
		t.Fatalf("stream chat: %v", err)
	}
	defer response.Body.Close()
	if authHeader != "Bearer secret" {
		t.Fatalf("unexpected auth header: %q", authHeader)
	}
	if received["session_id"] != "abc" || received["stream"] != true || received["model"] != "gpt-test" {
		t.Fatalf("unexpected payload: %#v", received)
	}
	messages, ok := received["messages"].([]any)
	if !ok || len(messages) != 1 {
		t.Fatalf("unexpected messages: %#v", received["messages"])
	}
}

func TestListSessionsDecodesContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer secret" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(writer, `{"sessions":[{"id":"abc","title":"Hello","updated_at":"2026-01-01T00:00:00Z"}]}`)
	}))
	defer server.Close()

	client := NewRuntimeChatClient(server.URL, server.URL, "gpt-test", "secret")
	sessions, err := client.ListSessions(context.Background(), false)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "abc" || sessions[0].Title != "Hello" {
		t.Fatalf("unexpected sessions: %#v", sessions)
	}
}

func TestReadSessionMapsNotFoundToNil(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewRuntimeChatClient(server.URL, server.URL, "gpt-test", "secret")
	detail, err := client.ReadSession(context.Background(), "missing", SessionHistoryOptions{Limit: 50})
	if err != nil {
		t.Fatalf("read session: %v", err)
	}
	if detail != nil {
		t.Fatalf("expected nil for missing session, got %#v", detail)
	}
}

func TestReadSessionDecodesMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasSuffix(request.URL.Path, "/v1/sessions/abc/messages") {
			t.Errorf("unexpected path: %s", request.URL.Path)
		}
		if request.URL.Query().Get("limit") != "50" || request.URL.Query().Get("before") != "cursor" {
			t.Errorf("unexpected query: %s", request.URL.RawQuery)
		}
		_, _ = io.WriteString(writer, `{"id":"abc","title":"Hello","messages":[{"role":"user","content":"hi","created_at":"2026-01-01T00:00:00Z"}]}`)
	}))
	defer server.Close()

	client := NewRuntimeChatClient(server.URL, server.URL, "gpt-test", "secret")
	detail, err := client.ReadSession(context.Background(), "abc", SessionHistoryOptions{Limit: 50, Before: "cursor"})
	if err != nil {
		t.Fatalf("read session: %v", err)
	}
	if detail == nil || len(detail.Messages) != 1 || detail.Messages[0].Content != "hi" {
		t.Fatalf("unexpected detail: %#v", detail)
	}
}

func TestSessionLifecycleRequestsUseRuntimeAuthorization(t *testing.T) {
	var methods []string
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		methods = append(methods, request.Method+" "+request.URL.Path)
		authHeader = request.Header.Get("Authorization")
		switch {
		case request.Method == http.MethodPatch:
			_, _ = io.WriteString(writer, `{"id":"abc","title":"Renamed"}`)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/export"):
			_, _ = io.WriteString(writer, `{"id":"abc","snapshot":{"messages":[]}}`)
		default:
			_, _ = io.WriteString(writer, `{}`)
		}
	}))
	defer server.Close()
	client := NewRuntimeChatClient(server.URL, server.URL, "gpt-test", "secret")
	if _, err := client.RenameSession(context.Background(), "abc", "Renamed"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := client.ArchiveSession(context.Background(), "abc", true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if err := client.ArchiveSession(context.Background(), "abc", false); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if exported, err := client.ExportSession(context.Background(), "abc"); err != nil || exported == nil || exported.ID != "abc" {
		t.Fatalf("export: exported=%#v err=%v", exported, err)
	}
	if err := client.DeleteSession(context.Background(), "abc"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if authHeader != "Bearer secret" {
		t.Fatalf("unexpected authorization: %q", authHeader)
	}
	want := []string{"PATCH /v1/sessions/abc", "POST /v1/sessions/abc/archive", "POST /v1/sessions/abc/restore", "GET /v1/sessions/abc/export", "DELETE /v1/sessions/abc"}
	if strings.Join(methods, ",") != strings.Join(want, ",") {
		t.Fatalf("requests = %#v, want %#v", methods, want)
	}
}
