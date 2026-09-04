package transport

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const maxWebSocketMessageSize = 1 << 20

// wsConn wraps gorilla/websocket so every terminal and progress endpoint shares
// RFC-compliant framing, fragmentation, masking, and control-frame handling.
type WSConn struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

type wsMsg struct {
	Step   string `json:"step"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Index  int    `json:"index"`
	Total  int    `json:"total"`
	Ts     string `json:"ts"`
}

const wsStatusRunning = "running"
const wsStatusSuccess = "success"
const wsStatusFailed = "failed"

const (
	WSStatusRunning = wsStatusRunning
	WSStatusSuccess = wsStatusSuccess
	WSStatusFailed  = wsStatusFailed
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

// wsUpgrade upgrades an HTTP connection using gorilla/websocket's default
// same-origin check.
// WSUpgrade upgrades an HTTP connection.
func WSUpgrade(w http.ResponseWriter, r *http.Request) (*WSConn, error) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(maxWebSocketMessageSize)
	return &WSConn{conn: conn}, nil
}

// writeJSON sends a text frame with JSON payload.
// WriteJSON sends a text frame containing a JSON value.
func (c *WSConn) WriteJSON(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

func (c *WSConn) writeFrame(messageType int, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(messageType, payload)
}

// Close closes the WebSocket connection with normal closure.
func (c *WSConn) Close() error {
	_ = c.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	return c.conn.Close()
}

// ReadFrame reads one complete application message. Gorilla handles client
// masking, fragmented messages, and Ping/Pong control frames internally.
func (c *WSConn) ReadFrame() ([]byte, error) {
	_, payload, err := c.conn.ReadMessage()
	return payload, err
}

// WriteFrame sends a binary frame to the client.
func (c *WSConn) WriteFrame(data []byte) error {
	return c.writeFrame(websocket.BinaryMessage, data)
}

// readClose waits until the peer closes the connection.
// ReadClose waits until the peer closes the connection.
func (c *WSConn) ReadClose() error {
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return err
		}
	}
}

// sendProgress sends a single progress message through the WebSocket.
// SendProgress sends one progress event through a shared WebSocket.
func SendProgress(conn *WSConn, total int, index int, step, label, status, detail, ts string) bool {
	msg := wsMsg{Step: step, Label: label, Status: status, Detail: detail, Index: index, Total: total, Ts: ts}
	if err := conn.WriteJSON(msg); err != nil {
		return false
	}
	return true
}
