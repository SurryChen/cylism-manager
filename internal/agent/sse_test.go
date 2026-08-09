package agent

import (
	"strings"
	"testing"
)

func TestForEachChatEventExtractsDeltasAndDone(t *testing.T) {
	stream := "data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\n" +
		"data: [DONE]\n\n"
	var events []ChatEvent
	if err := ForEachChatEvent(strings.NewReader(stream), func(event ChatEvent) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatalf("parse stream: %v", err)
	}
	if len(events) != 3 || events[0].Type != EventDelta || events[0].Content != "你" || events[1].Content != "好" || events[2].Type != EventDone {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestForEachChatEventSkipsCommentsAndEmptyPayloads(t *testing.T) {
	stream := ": keep-alive\n\ndata:\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"
	var events []ChatEvent
	if err := ForEachChatEvent(strings.NewReader(stream), func(event ChatEvent) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatalf("parse stream: %v", err)
	}
	if len(events) != 1 || events[0].Content != "ok" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestForEachChatEventReportsUpstreamError(t *testing.T) {
	stream := "data: {\"error\":{\"message\":\"模型超时\"}}\n\n"
	var events []ChatEvent
	if err := ForEachChatEvent(strings.NewReader(stream), func(event ChatEvent) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatalf("parse stream: %v", err)
	}
	if len(events) != 1 || events[0].Type != EventError || events[0].Message == "" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestForEachChatEventStopsAtDone(t *testing.T) {
	stream := "data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\n" +
		"data: [DONE]\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"b\"}}]}\n\n"
	var events []ChatEvent
	if err := ForEachChatEvent(strings.NewReader(stream), func(event ChatEvent) error {
		events = append(events, event)
		return nil
	}); err != nil {
		t.Fatalf("parse stream: %v", err)
	}
	if len(events) != 2 || events[1].Type != EventDone {
		t.Fatalf("expected done to terminate the stream: %#v", events)
	}
}
