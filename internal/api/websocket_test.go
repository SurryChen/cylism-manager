package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestWSConnReadFrameHandlesLargeTerminalPaste(t *testing.T) {
	payload := []byte(strings.Repeat("paste-data-", 32*1024))
	serverErr := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrade(w, r)
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		data, err := conn.ReadFrame()
		if err == nil && !bytes.Equal(data, payload) {
			err = fmt.Errorf("received payload differs from pasted data")
		}
		if err == nil {
			err = conn.WriteFrame(data)
		}
		serverErr <- err
	}))
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatal(err)
	}
	messageType, received, err := client.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if messageType != websocket.BinaryMessage || !bytes.Equal(received, payload) {
		t.Fatalf("unexpected terminal round trip: type=%d bytes=%d", messageType, len(received))
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}
