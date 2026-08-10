// Package agent defines the platform contract for runtime chat and session
// access. Manager acts as the authenticated proxy; the runtime owns session
// state in its workspace.
package agent

// Session is one runtime conversation in the platform contract.
type Session struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// Message is one conversation turn in the platform contract.
type Message struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at,omitempty"`
}

// SessionDetail is a session together with its message history.
type SessionDetail struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	UpdatedAt  string    `json:"updated_at,omitempty"`
	Messages   []Message `json:"messages"`
	HasMore    bool      `json:"has_more"`
	NextCursor string    `json:"next_cursor,omitempty"`
}

// SessionHistoryOptions bounds a history page and identifies the end of the
// preceding page through the opaque cursor returned by the runtime.
type SessionHistoryOptions struct {
	Limit  int
	Before string
}

// Event types emitted on the Manager chat SSE stream.
const (
	EventDelta = "delta"
	EventDone  = "done"
	EventError = "error"
)

// ChatEvent is one SSE payload in the platform contract.
type ChatEvent struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
	Message string `json:"message,omitempty"`
}
