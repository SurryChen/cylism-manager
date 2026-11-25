package transport

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/cylism/cylism-manager/internal/model"
)

// SyncLocalVolumeContext copies between two volumes validated by the local-path
// provisioner. Both paths must exist and resolve to separate directories.
func SyncLocalVolumeContext(ctx context.Context, server *model.Server, key []byte, source, target string, copyData bool) error {
	if !path.IsAbs(source) || !path.IsAbs(target) || path.Clean(source) == "/" || path.Clean(target) == "/" {
		return fmt.Errorf("本地卷目录无效")
	}
	command := `set -eu
src=$(sudo -n realpath -e -- ` + QuoteShell(source) + `)
dst=$(sudo -n realpath -e -- ` + QuoteShell(target) + `)
test "$src" = ` + QuoteShell(path.Clean(source)) + `
test "$dst" = ` + QuoteShell(path.Clean(target)) + `
case "$src/" in "$dst/"*) exit 3;; esac
case "$dst/" in "$src/"*) exit 3;; esac
sudo -n test -d "$src"
sudo -n test -d "$dst"
test "$(sudo -n stat -Lc %d:%i -- "$src")" != "$(sudo -n stat -Lc %d:%i -- "$dst")"
sudo -n rsync --version >/dev/null
`
	timeout := 20 * time.Second
	if copyData {
		timeout = 30 * time.Minute
		command += `sudo -n rsync -aHAX --numeric-ids --one-file-system --delete --checksum -- "$src/" "$dst/" >/dev/null
changes=$(sudo -n rsync -naHAX --numeric-ids --one-file-system --delete --checksum --itemize-changes -- "$src/" "$dst/")
test -z "$changes"
`
	}
	if _, err := SSHExecServerContext(ctx, timeout, server, key, command); err != nil {
		return fmt.Errorf("节点 %s 的卷同步失败，请检查目录、免密 sudo、rsync 和磁盘空间: %w", server.Name, err)
	}
	return nil
}
