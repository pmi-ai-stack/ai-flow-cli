package guard_test

import (
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/agenthost/guard"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

const (
	testHome = "/home/dev/.ai-flow"
	testRepo = "/home/dev/acme.billing.api"
	testKey  = "PROJ-1"
	otherKey = "OTHER-9"
)

var testNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

const testIdle = 8 * time.Hour

func freshActive(extra func(*handoff.Active)) *handoff.Active {
	a := &handoff.Active{
		Key:         testKey,
		RepoRoots:   []string{testRepo},
		LastEventAt: testNow.Add(-1 * time.Hour).Format(time.RFC3339),
	}
	if extra != nil {
		extra(a)
	}
	return a
}

func staleActive() *handoff.Active {
	return freshActive(func(a *handoff.Active) {
		a.LastEventAt = testNow.Add(-9 * time.Hour).Format(time.RFC3339)
	})
}

func evalWrite(path string, active *handoff.Active) agenthost.Decision {
	return guard.Evaluate(guard.Input{
		Event:  agenthost.HookEvent{Host: agenthost.HostClaudeCode, Event: agenthost.EventWrite, FilePath: path},
		Active: active, Home: testHome, Idle: testIdle, Now: testNow,
	})
}

func evalShell(command, cwd string, active *handoff.Active) agenthost.Decision {
	return guard.Evaluate(guard.Input{
		Event:  agenthost.HookEvent{Host: agenthost.HostClaudeCode, Event: agenthost.EventShell, Command: command, Cwd: cwd},
		Active: active, Home: testHome, Idle: testIdle, Now: testNow,
	})
}

func requireDeny(t *testing.T, d agenthost.Decision) {
	t.Helper()
	if d.Allow || d.Ask {
		t.Fatalf("Allow = %v, Ask = %v, want deny (reason=%q)", d.Allow, d.Ask, d.Reason)
	}
	if d.OwningCommand == "" {
		t.Error("a deny decision must name an owning command")
	}
	if d.UserMessage == "" || d.AgentMessage == "" {
		t.Error("a deny decision must carry both user_message and agent_message")
	}
}

func requireAllow(t *testing.T, d agenthost.Decision) {
	t.Helper()
	if !d.Allow {
		t.Fatalf("Allow = false, want allow (reason=%q, message=%q)", d.Reason, d.AgentMessage)
	}
}

// --- Row 1: ~/.ai-flow/<KEY>/** except the prose artifacts ---

func TestRow1_TicketDir(t *testing.T) {
	path := testHome + "/" + testKey + "/status.json"

	t.Run("deny", func(t *testing.T) {
		requireDeny(t, evalWrite(path, freshActive(nil)))
	})
	t.Run("allow_prose_artifact", func(t *testing.T) {
		requireAllow(t, evalWrite(testHome+"/"+testKey+"/plan.md", freshActive(nil)))
	})
	t.Run("allow_out_of_scope_different_ticket", func(t *testing.T) {
		requireAllow(t, evalWrite(testHome+"/"+otherKey+"/status.json", freshActive(nil)))
	})
	t.Run("allow_when_idle", func(t *testing.T) {
		requireAllow(t, evalWrite(path, staleActive()))
	})
	t.Run("allow_when_active_absent", func(t *testing.T) {
		requireAllow(t, evalWrite(path, nil))
	})
}

// --- Row 2: standards/, config.json, active.json — while any ticket is active ---

func TestRow2_StandardsConfigActive(t *testing.T) {
	for _, path := range []string{
		testHome + "/config.json",
		testHome + "/active.json",
		testHome + "/standards/acme-org@v3.2.0/manifest.json",
	} {
		t.Run(path, func(t *testing.T) {
			t.Run("deny", func(t *testing.T) {
				requireDeny(t, evalWrite(path, freshActive(nil)))
			})
			t.Run("allow_when_idle", func(t *testing.T) {
				requireAllow(t, evalWrite(path, staleActive()))
			})
			t.Run("allow_when_active_absent", func(t *testing.T) {
				requireAllow(t, evalWrite(path, nil))
			})
		})
	}
	t.Run("allow_out_of_scope", func(t *testing.T) {
		requireAllow(t, evalWrite(testHome+"/some-other-file.json", freshActive(nil)))
	})
}

// --- Row 3: ~/.ai-flow/cache/** — always ---

func TestRow3_Cache(t *testing.T) {
	path := testHome + "/cache/render/deadbeef/plan.json"

	t.Run("deny_with_active", func(t *testing.T) {
		requireDeny(t, evalWrite(path, freshActive(nil)))
	})
	t.Run("deny_when_idle", func(t *testing.T) {
		// Row 3's own When column is "always" — unlike every other row, the
		// idle rule (which only ever nils out Active) cannot make this one
		// visible, since it never consulted Active to begin with.
		requireDeny(t, evalWrite(path, staleActive()))
	})
	t.Run("deny_when_active_absent", func(t *testing.T) {
		requireDeny(t, evalWrite(path, nil))
	})
	t.Run("allow_out_of_scope", func(t *testing.T) {
		requireAllow(t, evalWrite(testHome+"/render/x.json", nil))
	})
}

// --- Row 4: <repo>/.ai-flow/**, inside an active ticket's repo ---

func TestRow4_Waivers(t *testing.T) {
	path := testRepo + "/.ai-flow/waivers.json"

	t.Run("deny", func(t *testing.T) {
		requireDeny(t, evalWrite(path, freshActive(nil)))
	})
	// Phase 33's SEC-3: the whole directory, not just waivers.json —
	// visual.json carries free argv run outside the allowlist, and the
	// repo-layer pack can override org checks.
	for _, other := range []string{"/.ai-flow/visual.json", "/.ai-flow/standards/standards.json", "/.ai-flow/new.json"} {
		t.Run("deny"+other, func(t *testing.T) {
			requireDeny(t, evalWrite(testRepo+other, freshActive(nil)))
		})
	}
	t.Run("allow_outside_dot_dir", func(t *testing.T) {
		requireAllow(t, evalWrite(testRepo+"/.ai-flowx/other.json", freshActive(nil)))
		requireAllow(t, evalWrite(testRepo+"/src/.ai-flow.go", freshActive(nil)))
	})
	t.Run("allow_when_idle", func(t *testing.T) {
		requireAllow(t, evalWrite(path, staleActive()))
	})
	t.Run("allow_when_active_absent", func(t *testing.T) {
		requireAllow(t, evalWrite(path, nil))
	})
}

// --- Row 5: tool configuration files named in active.json ---

func TestRow5_ToolConfig(t *testing.T) {
	configPath := testRepo + "/.golangci.yml"
	withConfig := func(a *handoff.Active) { a.ToolConfigFiles = []string{configPath} }

	t.Run("deny", func(t *testing.T) {
		requireDeny(t, evalWrite(configPath, freshActive(withConfig)))
	})
	t.Run("allow_out_of_scope_unlisted_file", func(t *testing.T) {
		requireAllow(t, evalWrite(testRepo+"/other.yml", freshActive(withConfig)))
	})
	t.Run("allow_when_idle", func(t *testing.T) {
		requireAllow(t, evalWrite(configPath, freshActive(func(a *handoff.Active) {
			withConfig(a)
			a.LastEventAt = testNow.Add(-9 * time.Hour).Format(time.RFC3339)
		})))
	})
	t.Run("allow_when_active_absent", func(t *testing.T) {
		requireAllow(t, evalWrite(configPath, nil))
	})
}

// --- Row 6: git commit/push/tag, gh pr — always denied in scope ---

func TestRow6_GitPublish(t *testing.T) {
	for _, cmd := range []string{"git commit -m wip", "git push origin feature/PROJ-1", "git tag v1.0.0", "gh pr create --fill"} {
		t.Run(cmd, func(t *testing.T) {
			t.Run("deny", func(t *testing.T) {
				requireDeny(t, evalShell(cmd, testRepo, freshActive(nil)))
			})
			t.Run("allow_when_idle", func(t *testing.T) {
				requireAllow(t, evalShell(cmd, testRepo, staleActive()))
			})
			t.Run("allow_when_active_absent", func(t *testing.T) {
				requireAllow(t, evalShell(cmd, testRepo, nil))
			})
		})
	}
	t.Run("allow_out_of_scope", func(t *testing.T) {
		requireAllow(t, evalShell("git push origin main", "/home/dev/unrelated-repo", freshActive(nil)))
	})
	t.Run("allow_read_only_git_verb", func(t *testing.T) {
		// Row 6 names publish verbs only — "git status" is not on the
		// deny list, and it also is not one of row 7's guarded tools.
		requireAllow(t, evalShell("git status", testRepo, freshActive(nil)))
	})
}

// --- Row 7: infra tools (no addon yet) + exec.Table's own check/test/mutation tools ---

func TestRow7_InfraTestMutationTools(t *testing.T) {
	t.Run("deny_infra_tool_no_addon", func(t *testing.T) {
		requireDeny(t, evalShell("kubectl apply -f deploy.yaml", testRepo, freshActive(nil)))
	})
	t.Run("allow_matches_exec_table", func(t *testing.T) {
		// "go test ./..." matches exec.Table's {Tool:"go",Verb:["test"]}
		// row exactly — the same invocation ai-flow's own `test run` would
		// perform, proven non-mutating by TestAllowlistNeverMutatesPersistentState.
		requireAllow(t, evalShell("go test ./...", testRepo, freshActive(nil)))
	})
	t.Run("deny_no_exec_table_match", func(t *testing.T) {
		// "go" is a row-7 tool (exec.Table names it for "test" only) —
		// "go build" does not match that row, so it is denied.
		requireDeny(t, evalShell("go build ./...", testRepo, freshActive(nil)))
	})
	t.Run("allow_when_idle", func(t *testing.T) {
		requireAllow(t, evalShell("kubectl apply -f deploy.yaml", testRepo, staleActive()))
	})
	t.Run("allow_when_active_absent", func(t *testing.T) {
		requireAllow(t, evalShell("kubectl apply -f deploy.yaml", testRepo, nil))
	})
	t.Run("allow_out_of_scope", func(t *testing.T) {
		requireAllow(t, evalShell("kubectl apply -f deploy.yaml", "/home/dev/unrelated-repo", freshActive(nil)))
	})
}

// --- hook_shape_unknown ---

func TestHookShapeUnknown(t *testing.T) {
	t.Run("write_without_path", func(t *testing.T) {
		d := evalWrite("", freshActive(nil))
		requireAllow(t, d)
		if d.Reason != "hook_shape_unknown" {
			t.Errorf("Reason = %q, want hook_shape_unknown", d.Reason)
		}
	})
	t.Run("unknown_event_kind", func(t *testing.T) {
		d := guard.Evaluate(guard.Input{
			Event:  agenthost.HookEvent{Host: agenthost.HostClaudeCode, Event: agenthost.EventUnknown},
			Active: freshActive(nil), Home: testHome, Idle: testIdle, Now: testNow,
		})
		requireAllow(t, d)
		if d.Reason != "hook_shape_unknown" {
			t.Errorf("Reason = %q, want hook_shape_unknown", d.Reason)
		}
	})
}

func TestRows_EightRows(t *testing.T) {
	rows := guard.Rows()
	if len(rows) != 8 {
		t.Fatalf("Rows() returned %d rows, want 8", len(rows))
	}
	for _, r := range rows {
		if r.ID == "" || r.Kind == "" || r.Scope == "" || r.When == "" || r.OwningCommand == "" {
			t.Errorf("row %+v has an empty field", r)
		}
	}
}

// --- Row 8: ai-flow stage approve — the developer's decision ---

// TestRow8_GateApproval: approving a gate asks the developer through
// the host, whatever the active ticket or cwd — the one command whose
// whole point is a human decision (2026-10-04 security review #2).
func TestRow8_GateApproval(t *testing.T) {
	asks := []string{
		"ai-flow stage approve PROJ-1 ship",
		"/usr/local/bin/ai-flow stage approve PROJ-1 plan",
		"ai-flow --home /tmp/h stage approve PROJ-1 impact",
		"AI_FLOW_X=1 ai-flow stage approve PROJ-1 ship",
		"cd /tmp && ai-flow stage approve PROJ-1 ship",
		"true;ai-flow stage approve PROJ-1 ship",
		`sh -c "ai-flow stage approve PROJ-1 ship"`,
		"ai-flow stage approve PROJ-1 ship | cat",
	}
	for _, cmd := range asks {
		for name, active := range map[string]*handoff.Active{"active": freshActive(nil), "no ticket": nil, "stale": staleActive()} {
			t.Run(name+"/"+cmd, func(t *testing.T) {
				d := evalShell(cmd, "/somewhere/else", active)
				if d.Allow || !d.Ask {
					t.Fatalf("Allow = %v, Ask = %v, want ask (reason=%q)", d.Allow, d.Ask, d.Reason)
				}
				if d.Reason != "row8_gate_approval" {
					t.Errorf("Reason = %q, want row8_gate_approval", d.Reason)
				}
				if d.UserMessage == "" || d.AgentMessage == "" || d.OwningCommand == "" {
					t.Errorf("an ask decision must carry messages and an owning command: %+v", d)
				}
			})
		}
	}

	allows := []string{
		"ai-flow stage start PROJ-1 ship",
		"ai-flow status PROJ-1",
		"ai-flow stage complete PROJ-1 ship",
		"echo approve the stage",
		"git log --grep 'stage approve'",
	}
	for _, cmd := range allows {
		t.Run("allow/"+cmd, func(t *testing.T) {
			requireAllow(t, evalShell(cmd, testRepo, freshActive(nil)))
		})
	}
}
