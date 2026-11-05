package transport

import (
	"bytes"
	"context"
	"fmt"
	"github.com/cylism/cylism-manager/internal/model"
	"io"
	"strings"
	"sync/atomic"
	"time"
)

func QuoteShell(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func CopyDirectoryContext(parent context.Context, source, target *model.Server, sourcePath, targetPath string, encKey []byte) (int64, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Minute)
	defer cancel()
	sourceCommandText := "sudo -n tar --numeric-owner -C " + QuoteShell(sourcePath) + " -cpf - ."
	targetCommandText := "sudo -n mkdir -p " + QuoteShell(targetPath) + " && sudo -n tar --numeric-owner -C " + QuoteShell(targetPath) + " -xpf -"
	var sourceErr, targetErr bytes.Buffer
	targetCommand, err := StartSSHCommandContext(ctx, 30*time.Minute, target, encKey, targetCommandText)
	if err != nil {
		return 0, err
	}
	defer targetCommand.Close()
	sourceCommand, err := StartSSHCommandContext(ctx, 30*time.Minute, source, encKey, sourceCommandText)
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
	copyErr := func() error {
		_, err := io.Copy(targetCommand.Stdin, io.TeeReader(sourceCommand.Stdout, countWriter{count: &copied}))
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
		return copied.Load(), fmt.Errorf("存储卷数据复制失败（已传输 %d bytes）: 源=%s；目标=%s", copied.Load(), streamCommandFailure(sourceRunErr, sourceErr.String()), streamCommandFailure(targetRunErr, targetErr.String()))
	}
	return copied.Load(), nil
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
