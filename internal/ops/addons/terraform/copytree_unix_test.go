//go:build unix

package terraform

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestCopyTreeSkipsNamedPipes: a FIFO in a source checkout must be
// skipped — os.ReadFile on one blocks forever, hanging `ops render`.
func TestCopyTreeSkipsNamedPipes(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.txt"), []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(src, "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo unsupported here: %v", err)
	}
	t.Cleanup(func() { // release a reader a failing run left blocked
		if f, err := os.OpenFile(fifo, os.O_RDWR|syscall.O_NONBLOCK, 0); err == nil {
			_ = f.Close()
		}
	})

	done := make(chan error, 1)
	go func() { done <- copyTree(src, dst) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("copyTree: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("copyTree blocked on a named pipe")
	}
	if got, err := os.ReadFile(filepath.Join(dst, "main.txt")); err != nil || string(got) != "kept" {
		t.Errorf("regular file not copied: %q, %v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "pipe")); err == nil {
		t.Error("the FIFO was copied into the rendering")
	}
}
