package server

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// TestChatManager_SetSelfURL_ConcurrentWithStartIsRaceFree regression-guards:
// SetSelfURL wrote m.selfURL without holding m.mu (unlike the neighboring
// SetSlashLLMFactory), and Start reads m.selfURL later without a lock too.
// Run under `go test -race`: this only proves anything with the race
// detector enabled — a plain run can't observe an unsynchronized read/write
// as a wrong value, only -race can catch the concurrent access itself.
func TestChatManager_SetSelfURL_ConcurrentWithStartIsRaceFree(t *testing.T) {
	m, configID := newForkTestManager(t)
	m.buildFunc = makeFakeChatBuildFunc("a1", nil, nil)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			m.SetSelfURL(fmt.Sprintf("http://race-%d", i))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			sess, err := m.Start(context.Background(), configID, "", nil, "")
			if err == nil {
				sess.Close()
			}
		}
	}()
	wg.Wait()
}
