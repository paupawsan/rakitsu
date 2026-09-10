//go:build !windows

package fs

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// An out-of-fence FIFO must be refused promptly. basePath is pinned as a
// handle before searchRoot runs, so a blocking open would hang this
// goroutine before the allowed-path check could reject the path at all.
func TestSearch_FIFOOutsideAllowed_DoesNotHang(t *testing.T) {
	outside := t.TempDir()
	allowed := t.TempDir()
	fifo := filepath.Join(outside, "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo unsupported here: %v", err)
	}

	tool := newFSTool("search", []string{allowed})
	done := make(chan error, 1)
	go func() {
		_, err := tool.searchFiles(context.Background(), fifo, "marker-fifo", "*")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an out-of-fence FIFO to be refused")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("search blocked opening an out-of-fence FIFO")
	}
}
