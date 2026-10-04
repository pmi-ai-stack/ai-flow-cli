package github

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

// halfOpenListener accepts TCP connections and never writes a byte —
// the server shape that hung `ai-flow ticket show`/`doctor` with no
// envelope before requests were bounded.
func halfOpenListener(t *testing.T) string {
	t.Helper()
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
	return "http://" + ln.Addr().String()
}

func TestReachableTimesOutAgainstHalfOpenServer(t *testing.T) {
	c := New("tok", http.DefaultTransport, nil).WithTimeout(200 * time.Millisecond)
	c.baseURL = halfOpenListener(t)

	start := time.Now()
	err := c.Reachable(context.Background())
	if err == nil {
		t.Fatal("Reachable returned nil against a server that never responds")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Reachable took %v, want it bounded by the 200ms timeout", elapsed)
	}
}

func TestNewIsBoundedByDefault(t *testing.T) {
	c := New("tok", http.DefaultTransport, nil)
	if c.httpClient.Timeout != DefaultTimeout || c.timeout != DefaultTimeout {
		t.Errorf("timeouts = %v/%v, want DefaultTimeout %v on both", c.httpClient.Timeout, c.timeout, DefaultTimeout)
	}
}

func TestTransportFailureIsToolFailed(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + ln.Addr().String()
	_ = ln.Close() // nothing listens: connection refused

	c := New("tok", http.DefaultTransport, nil)
	c.baseURL = base
	err = c.Reachable(context.Background())
	var env *envelope.Error
	if !errors.As(err, &env) || env.Code != "tool_failed" {
		t.Fatalf("Reachable against a refused connection = %v, want envelope.Error{tool_failed}", err)
	}
}
