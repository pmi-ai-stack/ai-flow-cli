package agenthost_test

import (
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost"
)

type stubAdapter struct{ host agenthost.Host }

func (s stubAdapter) Host() agenthost.Host { return s.host }
func (stubAdapter) Detect(string) (agenthost.Detection, bool, error) {
	return agenthost.Detection{}, false, nil
}
func (stubAdapter) ParseHook([]byte) (agenthost.HookEvent, error) { return agenthost.HookEvent{}, nil }
func (stubAdapter) EncodeHook(agenthost.Decision) ([]byte, error) { return nil, nil }
func (stubAdapter) HookConfig() string                            { return "" }
func (stubAdapter) SkillsDirs() []string                          { return nil }

func TestFromEnv(t *testing.T) {
	claude := stubAdapter{host: agenthost.HostClaudeCode}
	cursor := stubAdapter{host: agenthost.HostCursor}

	tests := []struct {
		name string
		env  []string
		want agenthost.Host
		ok   bool
	}{
		{"claude code set", []string{"CLAUDE_PLUGIN_ROOT=/plugin"}, agenthost.HostClaudeCode, true},
		{"cursor set", []string{"CURSOR_PLUGIN_ROOT=/plugin"}, agenthost.HostCursor, true},
		{"neither set", []string{"PATH=/usr/bin"}, "", false},
		{"both set", []string{"CLAUDE_PLUGIN_ROOT=/a", "CURSOR_PLUGIN_ROOT=/b"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := agenthost.FromEnv(tt.env, claude, cursor)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok && got.Host() != tt.want {
				t.Fatalf("host = %v, want %v", got.Host(), tt.want)
			}
		})
	}
}

func TestFixtureNames_ExcludesMalformed(t *testing.T) {
	for _, host := range []agenthost.Host{agenthost.HostClaudeCode, agenthost.HostCursor} {
		names, err := agenthost.FixtureNames(host)
		if err != nil {
			t.Fatalf("FixtureNames(%s): %v", host, err)
		}
		if len(names) == 0 {
			t.Fatalf("FixtureNames(%s): no fixtures found", host)
		}
		for _, n := range names {
			if n == "malformed.json" {
				t.Fatalf("FixtureNames(%s): malformed.json must be excluded", host)
			}
		}
	}
}

func TestFixture_Malformed(t *testing.T) {
	for _, host := range []agenthost.Host{agenthost.HostClaudeCode, agenthost.HostCursor} {
		data, err := agenthost.Fixture(host, "malformed.json")
		if err != nil {
			t.Fatalf("Fixture(%s, malformed.json): %v", host, err)
		}
		if len(data) == 0 {
			t.Fatalf("Fixture(%s, malformed.json): empty", host)
		}
	}
}
