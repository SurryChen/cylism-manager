package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const dataPrefix = "data:"

// upstreamDelta extracts the content delta from an OpenAI-compatible streaming
// chunk. Unknown or non-content chunks yield an empty string.
func upstreamDelta(payload string) (string, error) {
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return "", fmt.Errorf("解析上游 SSE 分片: %w", err)
	}
	if chunk.Error != nil {
		return "", fmt.Errorf("上游生成错误: %s", chunk.Error.Message)
	}
	if len(chunk.Choices) > 0 {
		return chunk.Choices[0].Delta.Content, nil
	}
	return "", nil
}

// ForEachChatEvent reads an SSE stream and invokes fn for every meaningful
// event: content deltas and the terminal done marker.
func ForEachChatEvent(reader io.Reader, fn func(ChatEvent) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, dataPrefix) {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, dataPrefix))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			if err := fn(ChatEvent{Type: EventDone}); err != nil {
				return err
			}
			return nil
		}
		delta, err := upstreamDelta(payload)
		if err != nil {
			if err := fn(ChatEvent{Type: EventError, Message: err.Error()}); err != nil {
				return err
			}
			continue
		}
		if delta == "" {
			continue
		}
		if err := fn(ChatEvent{Type: EventDelta, Content: delta}); err != nil {
			return err
		}
	}
	return scanner.Err()
}
