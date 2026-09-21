package envelope

import "testing"

// TestExitCodeTable enumerates every documented error.code (the full
// table this binary's envelope contract defines, in codeExitCode) and
// asserts it resolves to its documented exit code. Adding a code to that
// table without adding it here fails the build, and vice versa.
func TestExitCodeTable(t *testing.T) {
	cases := []struct {
		code string
		want ExitCode
	}{
		{"internal", ExitInternal},
		{"usage", ExitUsage},
		{"exec_not_allowed", ExitUsage},

		{"missing_artifact", ExitPrecondition},
		{"stage_not_allowed", ExitPrecondition},
		{"approval_required", ExitPrecondition},
		{"output_not_passed", ExitPrecondition},
		{"must_findings_open", ExitPrecondition},

		{"routing_ambiguous", ExitNeedsInput},
		{"dirty_worktree", ExitNeedsInput},
		{"base_diverged", ExitNeedsInput},
		{"missing_repo_instructions", ExitNeedsInput},
		{"tool_missing", ExitNeedsInput},
		{"mutation_below_threshold", ExitNeedsInput},
		{"env_ambiguous", ExitNeedsInput},
		{"live_credentials_missing", ExitNeedsInput},
		{"waiver_confirmation", ExitNeedsInput},

		{"tracker_auth", ExitProviderFailure},
		{"tracker_not_found", ExitProviderFailure},
		{"host_auth", ExitProviderFailure},
		{"rate_limited", ExitProviderFailure},
		{"live_auth", ExitProviderFailure},
		{"tool_failed", ExitProviderFailure},
		{"pack_unreachable", ExitProviderFailure},

		{"invalid_result", ExitSchemaInvalid},
		{"unknown_report_format", ExitSchemaInvalid},
		{"pack_invalid", ExitSchemaInvalid},
		{"check_runner_missing", ExitSchemaInvalid},
		{"config_invalid", ExitSchemaInvalid},
	}

	if len(cases) != len(codeExitCode) {
		t.Fatalf("table has %d codes but this test only checks %d — every documented code must be covered", len(codeExitCode), len(cases))
	}

	seen := make(map[string]bool, len(cases))
	for _, c := range cases {
		if seen[c.code] {
			t.Fatalf("duplicate case for code %q", c.code)
		}
		seen[c.code] = true

		got, ok := ExitCodeFor(c.code)
		if !ok {
			t.Errorf("ExitCodeFor(%q): unknown code", c.code)
			continue
		}
		if got != c.want {
			t.Errorf("ExitCodeFor(%q) = %d, want %d", c.code, got, c.want)
		}
	}
}

func TestExitCodeForUnknown(t *testing.T) {
	if _, ok := ExitCodeFor("not_a_real_code"); ok {
		t.Error("ExitCodeFor(unknown code) reported ok=true")
	}
}
