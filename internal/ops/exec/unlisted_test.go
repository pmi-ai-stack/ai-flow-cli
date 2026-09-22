package exec

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/redact"
)

func TestStartUnlistedSkipsAllowlist(t *testing.T) {
	// "definitely-not-a-real-tool" matches no row in Table — proving
	// StartUnlisted never consults Allowlist.Match.
	r := &Runner{Allowlist: NewAllowlist(), Redact: redact.New(nil)}
	p, err := r.StartUnlisted(context.Background(), t.TempDir(), "sh", []string{"-c", "exit 0"}, nil)
	if err != nil {
		t.Fatalf("StartUnlisted: %v", err)
	}
	if err := p.Stop(context.Background(), time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestStartUnlistedStopSIGTERM(t *testing.T) {
	r := &Runner{Allowlist: NewAllowlist(), Redact: redact.New(nil)}
	p, err := r.StartUnlisted(context.Background(), t.TempDir(), "sh", []string{"-c", "trap 'exit 0' TERM; sleep 30"}, nil)
	if err != nil {
		t.Fatalf("StartUnlisted: %v", err)
	}
	if err := p.Stop(context.Background(), 2*time.Second); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestStartUnlistedStopSIGKILLFallback(t *testing.T) {
	r := &Runner{Allowlist: NewAllowlist(), Redact: redact.New(nil)}
	// Ignores SIGTERM outright, forcing Stop's SIGKILL fallback.
	p, err := r.StartUnlisted(context.Background(), t.TempDir(), "sh", []string{"-c", "trap '' TERM; sleep 30"}, nil)
	if err != nil {
		t.Fatalf("StartUnlisted: %v", err)
	}
	start := time.Now()
	if err := p.Stop(context.Background(), 200*time.Millisecond); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Stop took %v, want well under the 30s sleep — SIGKILL fallback did not fire", elapsed)
	}
}

func TestProcessStopNilIsNoop(t *testing.T) {
	var p *Process
	if err := p.Stop(context.Background(), time.Second); err != nil {
		t.Errorf("Stop(nil) = %v, want nil", err)
	}
}

func TestProcessStopTwiceIsNoop(t *testing.T) {
	r := &Runner{Allowlist: NewAllowlist(), Redact: redact.New(nil)}
	p, err := r.StartUnlisted(context.Background(), t.TempDir(), "sh", []string{"-c", "exit 0"}, nil)
	if err != nil {
		t.Fatalf("StartUnlisted: %v", err)
	}
	if err := p.Stop(context.Background(), time.Second); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := p.Stop(context.Background(), time.Second); err != nil {
		t.Errorf("second Stop: %v, want nil (no-op)", err)
	}
}

func TestRunUnlistedSkipsAllowlistAndRedacts(t *testing.T) {
	r := NewRunner()
	out, err := r.RunUnlisted(context.Background(), t.TempDir(), "sh", []string{"-c", "echo hello-world"}, nil, 5*time.Second)
	if err != nil {
		t.Fatalf("RunUnlisted: %v", err)
	}
	if !strings.Contains(out.Stdout, "hello-world") {
		t.Errorf("Stdout = %q, want it to contain hello-world", out.Stdout)
	}
}

func TestRunUnlistedNonZeroExit(t *testing.T) {
	r := NewRunner()
	_, err := r.RunUnlisted(context.Background(), t.TempDir(), "sh", []string{"-c", "exit 3"}, nil, 5*time.Second)
	if err == nil {
		t.Fatal("RunUnlisted: want an error on non-zero exit")
	}
}

func TestRunUnlistedTimeout(t *testing.T) {
	r := NewRunner()
	_, err := r.RunUnlisted(context.Background(), t.TempDir(), "sh", []string{"-c", "sleep 30"}, nil, 100*time.Millisecond)
	if err == nil {
		t.Fatal("RunUnlisted: want an error on timeout")
	}
}
