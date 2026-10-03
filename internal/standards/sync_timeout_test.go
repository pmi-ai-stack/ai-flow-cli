package standards

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

// TestSyncTimeoutAgainstHalfOpenRemote proves SyncOptions.Timeout
// bounds a remote source whose server accepts and never answers:
// pack_unreachable within the deadline, and no cache directory left
// behind for hook session-start's "absent" check to mistake for a
// synced pack.
func TestSyncTimeoutAgainstHalfOpenRemote(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var conns []net.Conn
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			conns = append(conns, c)
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		<-done
		for _, c := range conns {
			_ = c.Close()
		}
	})

	home := t.TempDir()
	src := Source{ID: "slow", Repo: "http://" + ln.Addr().String() + "/standards.git", Ref: "v1.0.0"}

	start := time.Now()
	_, err = Sync(context.Background(), SyncOptions{Home: home, Sources: []Source{src}, Timeout: 300 * time.Millisecond})
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("Sync took %v, want it bounded by the 300ms per-source timeout", elapsed)
	}
	var envErr *envelope.Error
	if !errors.As(err, &envErr) || envErr.Code != "pack_unreachable" {
		t.Fatalf("err = %v, want pack_unreachable", err)
	}
	if _, statErr := os.Stat(CacheDir(home, src.ID, src.Ref)); !os.IsNotExist(statErr) {
		t.Errorf("cache dir exists after a timed-out sync (stat err %v), want absent", statErr)
	}
}
