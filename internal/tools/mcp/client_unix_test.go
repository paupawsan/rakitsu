//go:build !windows

package mcp

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestStdioClient_OversizedLineKillsSubprocess regression-tests the fix for
// a >1MB response line breaking the read loop (correctly) but leaving the
// subprocess itself running unmanaged. Reaps the process directly via
// Process.Wait (bounded by a timeout) rather than a kill(pid, 0) liveness
// poll, since a killed-but-not-yet-reaped child is a zombie that would
// still answer kill(pid, 0) as "alive" and give a false failure.
func TestStdioClient_OversizedLineKillsSubprocess(t *testing.T) {
	client, err := NewStdioClient(os.Args[0], nil, map[string]string{helperProcessEnvVar: "1"}, 5)
	if err != nil {
		t.Fatalf("NewStdioClient: %v", err)
	}
	defer client.Close()

	if _, err := client.send(context.Background(), "trigger_oversized", nil); err == nil {
		t.Fatal("expected an error from an oversized response line")
	}

	waitDone := make(chan error, 1)
	go func() {
		_, waitErr := client.cmd.Process.Wait()
		waitDone <- waitErr
	}()
	select {
	case <-waitDone:
		// Reaped — confirms the read loop's own defer actually killed the
		// subprocess instead of leaving it running until Close() (which
		// might never be called by a real caller that just gives up on a
		// dead client) got around to it.
	case <-time.After(2 * time.Second):
		t.Error("subprocess was not reaped within 2s of the read loop exiting — looks like it was never killed")
	}
}
