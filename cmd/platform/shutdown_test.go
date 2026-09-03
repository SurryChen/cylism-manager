package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type shutdownServerFake struct {
	calls *[]string
	err   error
}

func (f shutdownServerFake) Shutdown(context.Context) error {
	*f.calls = append(*f.calls, "shutdown")
	return f.err
}

type backgroundStopperFake struct{ calls *[]string }

func (f backgroundStopperFake) Stop() { *f.calls = append(*f.calls, "stop") }
func (f backgroundStopperFake) Wait() { *f.calls = append(*f.calls, "wait") }

func TestShutdownPlatformStopsAndWaitsForBackgroundAfterHTTPShutdown(t *testing.T) {
	calls := []string{}
	wantErr := errors.New("shutdown failed")
	err := shutdownPlatform(context.Background(), shutdownServerFake{calls: &calls, err: wantErr}, backgroundStopperFake{calls: &calls})
	if !errors.Is(err, wantErr) {
		t.Fatalf("shutdown error = %v, want %v", err, wantErr)
	}
	if want := []string{"shutdown", "stop", "wait"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("shutdown order = %#v, want %#v", calls, want)
	}
}
