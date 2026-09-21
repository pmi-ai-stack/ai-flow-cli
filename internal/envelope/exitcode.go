package envelope

// ExitCode is one of the seven documented exit codes (0-6).
type ExitCode int

const (
	ExitOK              ExitCode = 0
	ExitInternal        ExitCode = 1
	ExitUsage           ExitCode = 2
	ExitPrecondition    ExitCode = 3
	ExitNeedsInput      ExitCode = 4
	ExitProviderFailure ExitCode = 5
	ExitSchemaInvalid   ExitCode = 6
)

// codeExitCode maps every documented error.code to its exit code. This
// is the full table from the spec's "Go CLI" exit-code section; an
// error.code used anywhere in the codebase that is missing here is a
// failing build, not a silent exit 1 — see TestExitCodeTable.
var codeExitCode = map[string]ExitCode{
	"internal":         ExitInternal,
	"usage":            ExitUsage,
	"exec_not_allowed": ExitUsage,

	"missing_artifact":   ExitPrecondition,
	"stage_not_allowed":  ExitPrecondition,
	"approval_required":  ExitPrecondition,
	"output_not_passed":  ExitPrecondition,
	"must_findings_open": ExitPrecondition,

	"routing_ambiguous":         ExitNeedsInput,
	"dirty_worktree":            ExitNeedsInput,
	"base_diverged":             ExitNeedsInput,
	"missing_repo_instructions": ExitNeedsInput,
	"tool_missing":              ExitNeedsInput,
	"mutation_below_threshold":  ExitNeedsInput,
	"env_ambiguous":             ExitNeedsInput,
	"live_credentials_missing":  ExitNeedsInput,
	"waiver_confirmation":       ExitNeedsInput,

	"tracker_auth":      ExitProviderFailure,
	"tracker_not_found": ExitProviderFailure,
	"host_auth":         ExitProviderFailure,
	"rate_limited":      ExitProviderFailure,
	"live_auth":         ExitProviderFailure,
	"tool_failed":       ExitProviderFailure,
	"pack_unreachable":  ExitProviderFailure,

	"invalid_result":        ExitSchemaInvalid,
	"unknown_report_format": ExitSchemaInvalid,
	"pack_invalid":          ExitSchemaInvalid,
	"check_runner_missing":  ExitSchemaInvalid,
}

// ExitCodeFor returns the exit code for a documented error.code and
// reports whether the code is known.
func ExitCodeFor(code string) (ExitCode, bool) {
	ec, ok := codeExitCode[code]
	return ec, ok
}

// KnownCodes returns every documented error.code, for tests that must
// enumerate the full table.
func KnownCodes() []string {
	codes := make([]string, 0, len(codeExitCode))
	for c := range codeExitCode {
		codes = append(codes, c)
	}
	return codes
}
