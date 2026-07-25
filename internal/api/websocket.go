package api

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
)

var wsMagicGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// wsConn wraps a raw net.Conn as a minimal WebSocket connection.
type wsConn struct {
	conn   net.Conn
	reader *bufio.Reader
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

// wsUpgrade upgrades an HTTP connection to WebSocket.
func wsUpgrade(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, errors.New("missing Sec-WebSocket-Key")
	}

	accept := base64.StdEncoding.EncodeToString(
		sha1Sum([]byte(key + wsMagicGUID)))

	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("server does not support hijacking")
	}
	conn, bufrw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}

	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := bufrw.WriteString(resp); err != nil {
		conn.Close()
		return nil, err
	}
	if err := bufrw.Flush(); err != nil {
		conn.Close()
		return nil, err
	}

	return &wsConn{conn: conn, reader: bufrw.Reader}, nil
}

// writeJSON sends a text frame with JSON payload.
func (c *wsConn) writeJSON(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.writeFrame(0x1, data) // text frame
}

// writeFrame sends a WebSocket frame.
func (c *wsConn) writeFrame(opcode byte, payload []byte) error {
	length := len(payload)
	buf := make([]byte, 0, 2+length)
	buf = append(buf, 0x80|opcode) // FIN + opcode

	if length < 126 {
		buf = append(buf, byte(length))
	} else if length < 65536 {
		buf = append(buf, 126, byte(length>>8), byte(length))
	} else {
		buf = append(buf, 127)
		for i := 7; i >= 0; i-- {
			buf = append(buf, byte(length>>(8*i)))
		}
	}
	buf = append(buf, payload...)
	_, err := c.conn.Write(buf)
	return err
}

// Close closes the WebSocket connection with normal closure.
func (c *wsConn) Close() error {
	closeFrame := []byte{0x88, 0x00} // FIN + close opcode, no payload
	c.conn.Write(closeFrame)
	return c.conn.Close()
}

// readClose waits for close frame from client.
func (c *wsConn) readClose() error {
	for {
		// Read first 2 bytes
		hdr := make([]byte, 2)
		if _, err := c.reader.Read(hdr); err != nil {
			return err
		}
		if (hdr[0]>>4) == 8 { // close frame
			return nil
		}
		length := int64(hdr[1] & 0x7f)
		if length == 126 {
			ext := make([]byte, 2)
			c.reader.Read(ext)
			length = int64(ext[0])<<8 | int64(ext[1])
		} else if length == 127 {
			ext := make([]byte, 8)
			c.reader.Read(ext)
			length = int64(ext[0])<<56 | int64(ext[1])<<48 | int64(ext[2])<<40 | int64(ext[3])<<32 |
				int64(ext[4])<<24 | int64(ext[5])<<16 | int64(ext[6])<<8 | int64(ext[7])
		}
		// Skip masked payload (4 mask bytes + payload)
		skip := make([]byte, 4+length)
		c.reader.Read(skip)
	}
}

func sha1Sum(data []byte) []byte {
	h := sha1.New()
	h.Write(data)
	return h.Sum(nil)
}

// sendProgress sends a single progress message through the WebSocket.
func sendProgress(conn *wsConn, total int, index int, step, label, status, detail, ts string) bool {
	msg := wsMsg{Step: step, Label: label, Status: status, Detail: detail, Index: index, Total: total, Ts: ts}
	if err := conn.writeJSON(msg); err != nil {
		return false
	}
	return true
}
