package sqlitemem

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Many handles on one file must wait for each other instead of failing with SQLITE_BUSY.
func TestConcurrentOpenDoesNotReturnBusy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := Open(path, nil)
			if err != nil {
				errs <- err
				return
			}
			time.Sleep(10 * time.Millisecond)
			_ = s.Close()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("open failed: %v", err)
	}
}
