package cluster

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestWaitSpeedListenerDelayedReady(t *testing.T) {
	output := newSpeedListenerOutput()
	done := make(chan error)
	result := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() { result <- waitSpeedListener(ctx, output, done) }()
	select {
	case err := <-result:
		t.Fatalf("listener considered ready without a listening message: %v", err)
	case <-time.After(650 * time.Millisecond):
	}
	_, _ = output.Write([]byte("Server listen"))
	_, _ = output.Write([]byte("ing on 5201 (test #1)\n"))
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestWaitSpeedListenerPreservesErrorAndCancellation(t *testing.T) {
	output := newSpeedListenerOutput()
	_, _ = output.Write([]byte("iperf3: error - unable to start listener: Address already in use\n"))
	err := waitSpeedListener(context.Background(), output, make(chan error))
	if err == nil || !strings.Contains(err.Error(), "Address already in use") || !strings.Contains(err.Error(), "iperf3.service") {
		t.Fatalf("lost listener diagnostic: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitSpeedListener(ctx, newSpeedListenerOutput(), make(chan error)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
	ended := make(chan error, 1)
	ended <- errors.New("SSH connection lost")
	err = waitSpeedListener(context.Background(), newSpeedListenerOutput(), ended)
	if err == nil || !strings.Contains(err.Error(), "SSH connection lost") {
		t.Fatalf("lost SSH diagnostic: %v", err)
	}
}

// Execute the actual remote shell wrapper locally with an iperf3 substitute
// that owns a real TCP socket. This detects orphaned children and accidental
// termination of somebody else's listener, rather than checking shell strings.
func TestSpeedListenerProcessLifecycle(t *testing.T) {
	dir := t.TempDir()
	helper, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for name, script := range map[string]string{
		"iperf3":  "#!/bin/sh\nexec \"$CYLISM_SPEED_TEST_HELPER\" -test.run=^TestSpeedListenerProcessHelper$ -- \"$@\"\n",
		"timeout": "#!/bin/sh\nshift 3\nexec \"$@\"\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	start := func(port int) (io.WriteCloser, <-chan error, *speedListenerOutput) {
		t.Helper()
		command := exec.Command("sh", "-c", speedListenerCommand(port))
		command.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "CYLISM_SPEED_TEST_HELPER="+helper, "CYLISM_SPEED_TEST_PROCESS=1")
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		input, err := command.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		output := newSpeedListenerOutput()
		command.Stdout, command.Stderr = output, output
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		var killOnce sync.Once
		stop := func() { killOnce.Do(func() { _ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }) }
		t.Cleanup(func() { _ = input.Close(); stop() })
		go func() { done <- command.Wait(); close(done) }()
		return input, done, output
	}
	t.Run("disconnect releases owned port", func(t *testing.T) {
		port := availableSpeedTestPort(t)
		input, done, output := start(port)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := waitSpeedListener(ctx, output, done); err != nil {
			t.Fatal(err)
		}
		_ = input.Close()
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("remote process survived stdin EOF")
		}
		listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			t.Fatalf("owned listener leaked: %v", err)
		}
		_ = listener.Close()
	})
	t.Run("completed one-shot exits with stdin still open", func(t *testing.T) {
		port := availableSpeedTestPort(t)
		input, done, output := start(port)
		defer input.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := waitSpeedListener(ctx, output, done); err != nil {
			t.Fatal(err)
		}
		connection, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_ = connection.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("successful listener exit: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("completed listener left wrapper alive")
		}
	})
	t.Run("bind failure preserves existing listener", func(t *testing.T) {
		existing, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer existing.Close()
		input, done, output := start(existing.Addr().(*net.TCPAddr).Port)
		defer input.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = waitSpeedListener(ctx, output, done)
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "address already in use") {
			t.Fatalf("bind error = %v", err)
		}
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("failed startup left wrapper alive")
		}
		connection, err := net.DialTimeout("tcp", existing.Addr().String(), time.Second)
		if err != nil {
			t.Fatalf("existing listener was affected: %v", err)
		}
		_ = connection.Close()
	})
}

func availableSpeedTestPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

func TestSpeedListenerProcessHelper(t *testing.T) {
	if os.Getenv("CYLISM_SPEED_TEST_PROCESS") != "1" {
		return
	}
	port := os.Args[len(os.Args)-1]
	if _, err := strconv.Atoi(port); err != nil {
		os.Exit(2)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:"+port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "iperf3: error - %v\n", err)
		os.Exit(1)
	}
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	completed := make(chan struct{})
	go func() {
		connection, err := listener.Accept()
		if err == nil {
			_ = connection.Close()
		}
		close(completed)
	}()
	fmt.Printf("Server listening on %s (test #1)\n", port)
	select {
	case <-quit:
	case <-completed:
	}
	_ = listener.Close()
	os.Exit(0)
}
