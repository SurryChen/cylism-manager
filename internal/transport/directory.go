package transport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/cylism/cylism-manager/internal/model"
	"io"
	"strings"
	"sync/atomic"
	"time"
)

func QuoteShell(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func CopyDirectoryContext(parent context.Context, source, target *model.Server, sourcePath, targetPath string, encKey []byte) (int64, error) {
	return CopyDirectoryControlledContext(parent, source, target, sourcePath, targetPath, encKey, nil)
}

type DirectoryCopyControl interface {
	Wait(context.Context) error
	Paused() bool
	Progress(int64)
}

func CopyDirectoryControlledContext(parent context.Context, source, target *model.Server, sourcePath, targetPath string, encKey []byte, control DirectoryCopyControl) (int64, error) {
	// Keep slow, progressing copies running for as long as their owner permits.
	// The progress watcher below still cancels stalled streams.
	ctx, cancel := context.WithCancelCause(parent)
	defer cancel(nil)
	sourceCommandText := "sudo -n tar --numeric-owner -C " + QuoteShell(sourcePath) + " -cpf - ."
	targetCommandText := "sudo -n mkdir -p " + QuoteShell(targetPath) + " && sudo -n tar --numeric-owner -C " + QuoteShell(targetPath) + " -xpf -"
	var sourceErr, targetErr bytes.Buffer
	targetCommand, err := StartSSHCommandContext(ctx, 30*time.Second, target, encKey, targetCommandText)
	if err != nil {
		return 0, err
	}
	defer targetCommand.Close()
	sourceCommand, err := StartSSHCommandContext(ctx, 30*time.Second, source, encKey, sourceCommandText)
	if err != nil {
		return 0, err
	}
	defer sourceCommand.Close()
	sourceStderrDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&sourceErr, sourceCommand.Stderr)
		close(sourceStderrDone)
	}()
	targetStderrDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&targetErr, targetCommand.Stderr)
		close(targetStderrDone)
	}()
	var copied atomic.Int64
	stopProgressWatch := watchControlledTransferProgress(ctx, &copied, cancel, 3*time.Minute, control)
	defer stopProgressWatch()
	copyErr := func() error {
		var reader io.Reader = sourceCommand.Stdout
		if control != nil {
			reader = &controlledReader{ctx: ctx, reader: reader, control: control}
		}
		_, err := io.Copy(&transferWriter{writer: targetCommand.Stdin, count: &copied, control: control}, reader)
		closeErr := targetCommand.Stdin.Close()
		if err != nil {
			return err
		}
		return closeErr
	}()
	if copyErr != nil {
		sourceCommand.Close()
		targetCommand.Close()
	}
	// Wait for the target first: it owns the reader side of source stdout. Calling
	// source Wait first can truncate a large tar stream.
	targetRunErr := targetCommand.Wait()
	if targetRunErr != nil {
		sourceCommand.Close()
	}
	sourceRunErr := sourceCommand.Wait()
	<-sourceStderrDone
	<-targetStderrDone
	if copyErr != nil || sourceRunErr != nil || targetRunErr != nil {
		return copied.Load(), fmt.Errorf("存储卷数据复制失败（已传输 %d bytes）: %w；源=%s；目标=%s", copied.Load(), errors.Join(context.Cause(ctx), copyErr, sourceRunErr, targetRunErr), streamCommandFailure(sourceRunErr, sourceErr.String()), streamCommandFailure(targetRunErr, targetErr.String()))
	}
	return copied.Load(), nil
}

func watchTransferProgress(ctx context.Context, copied *atomic.Int64, cancel context.CancelCauseFunc, idleTimeout time.Duration) func() {
	return watchControlledTransferProgress(ctx, copied, cancel, idleTimeout, nil)
}

func watchControlledTransferProgress(ctx context.Context, copied *atomic.Int64, cancel context.CancelCauseFunc, idleTimeout time.Duration, control DirectoryCopyControl) func() {
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(idleTimeout / 4)
		defer ticker.Stop()
		previous, lastProgress := copied.Load(), time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				current := copied.Load()
				if control != nil && control.Paused() {
					previous, lastProgress = current, time.Now()
					continue
				}
				if current != previous {
					previous, lastProgress = current, time.Now()
				} else if time.Since(lastProgress) >= idleTimeout {
					cancel(fmt.Errorf("复制连续 %s 无数据进展，停止传输并回退", idleTimeout))
					return
				}
			}
		}
	}()
	return func() { close(stop); <-done }
}

type controlledReader struct {
	ctx     context.Context
	reader  io.Reader
	control DirectoryCopyControl
}

func (r *controlledReader) Read(p []byte) (int, error) {
	if err := r.control.Wait(r.ctx); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

type transferWriter struct {
	writer  io.Writer
	count   *atomic.Int64
	control DirectoryCopyControl
}

func (w *transferWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	copied := w.count.Add(int64(n))
	if w.control != nil {
		w.control.Progress(copied)
	}
	return n, err
}

func streamCommandFailure(runErr error, output string) string {
	diagnostic := streamCommandDiagnostic(output)
	if diagnostic == "" && runErr == nil {
		return "无错误"
	}
	if diagnostic == "" {
		return runErr.Error()
	}
	if runErr == nil {
		return diagnostic
	}
	return runErr.Error() + ": " + diagnostic
}

func streamCommandDiagnostic(output string) string {
	lines := strings.Split(output, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Warning: Permanently added ") {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, " ")
}

type countWriter struct{ count *atomic.Int64 }

func (w countWriter) Write(value []byte) (int, error) {
	w.count.Add(int64(len(value)))
	return len(value), nil
}
