package envelope

import (
	"cmp"
	"slices"
)

// Explanation is what `ai-flow explain <code>` prints: what an
// error.code means and what the caller should do about it. The exit
// code is derived from codeExitCode, never stored a second time.
type Explanation struct {
	Code     string   `json:"code"`
	ExitCode ExitCode `json:"exit_code"`
	Meaning  string   `json:"meaning"`
	Recovery string   `json:"recovery"`
	// Reserved marks a code the spec's exit-code table documents but no
	// command emits today.
	Reserved bool `json:"reserved,omitempty"`
}

type explanationText struct {
	meaning, recovery string
	reserved          bool
}

// explanationTexts holds one entry per codeExitCode key, written from
// the code's emission sites. TestEveryCodeIsExplained fails the build
// when the two maps drift apart.
var explanationTexts = map[string]explanationText{
	"internal": {
		meaning:  "An unexpected failure inside ai-flow itself: a bug, or an error no command classified. It is not caused by the ticket, the configuration or a provider.",
		recovery: "Halt and show error.message. Do not retry blindly; report the command and the message.",
	},
	"usage": {
		meaning:  "The command was called wrongly: an unknown or missing argument or flag, a malformed ticket key, or a value that is not allowed (for example --stage ship with a directory).",
		recovery: "Fix the call; this is a caller or skill bug. Check the command's arguments against `ai-flow <noun> <verb> --help`.",
	},
	"exec_not_allowed": {
		meaning:  "A subprocess was requested that is not on the execution allowlist. Every tool ai-flow runs goes through that table; `ai-flow ops allowlist` prints it.",
		recovery: "Use the ai-flow command the message names instead of running the tool directly. Do not try to work around the allowlist.",
	},

	"missing_artifact": {
		meaning:  "A file the command needs does not exist yet: config.json before `config init`, or the result of an earlier stage (the message names it).",
		recovery: "Run the command or stage that writes that file, then retry.",
	},
	"stage_not_allowed": {
		meaning:  "The stage is not in a state that accepts this transition, for example completing a stage that was never started.",
		recovery: "Run `ai-flow status KEY` and follow next_skill. Do not skip or repeat stages.",
	},
	"approval_required": {
		meaning:  "git commit, git push or pr create was attempted before the developer approved the ship stage.",
		recovery: "Ask the developer for the ship approval. Only after they give it, run `ai-flow stage approve KEY ship`.",
	},
	"output_not_passed": {
		meaning:  "Ship approval was requested before the output stage passed, so there is no summary to approve.",
		recovery: "Run `ai-flow summary render KEY` and complete the output stage, then request ship approval again.",
	},
	"must_findings_open": {
		meaning:  "checks-results.json still holds an open must-severity finding. Completing the checks stage and approving ship are both refused while one is open.",
		recovery: "Fix each must finding at the source and re-run `ai-flow standards check KEY`. A finding can be waived only with the developer's confirmation, and never for the secrets check.",
	},

	"routing_ambiguous": {
		meaning:  "More than one repository could own the ticket, so ai-flow will not choose for the developer.",
		recovery: "Ask needs_input.question with its options, then run needs_input.resume_with (`ai-flow repo confirm`).",
	},
	"dirty_worktree": {
		meaning:  "The repository has uncommitted changes, so ai-flow will not create or switch the ticket branch.",
		recovery: "Ask the developer to commit, stash or discard the changes, then retry.",
	},
	"base_diverged": {
		meaning:  "The local base branch has commits that are not on its origin counterpart, so a branch cut from it would carry unpublished work.",
		recovery: "Ask the developer to resolve the local base branch (push or reset it), then retry.",
	},
	"missing_repo_instructions": {
		meaning:  "The repository has neither AGENTS.md nor CLAUDE.md, the file stage guidance is injected into. (`standards check <dir>` raises this only with --require-instructions.)",
		recovery: "Ask the developer to add an instructions file to the repository, then retry.",
	},
	"tool_missing": {
		meaning:  "Something the stage needs is not set up in the repository: an e2e suite, a mutation tool, or the dev_server.command or capture_command in .ai-flow/visual.json.",
		recovery: "Ask the developer whether to install or configure it, or to skip the stage with a reason. Do not install tools on your own.",
	},
	"mutation_below_threshold": {
		meaning:  "A repository's mutation score is below its configured threshold.",
		recovery: "Ask needs_input.question: add tests for the surviving mutants, mark genuine equivalents, or accept the score with a reason. Then run needs_input.resume_with.",
	},
	"env_ambiguous": {
		meaning:  "A target's path matches the naming convention of more than one configured environment; error.details lists the candidates.",
		recovery: "Ask the developer which environment is meant, or add the target to ops.target_map in config.json so it is no longer a guess.",
	},
	"live_credentials_missing": {
		meaning:  "`ops dry-run` needs live credentials for the target's environment to compute a changeset, and none are available.",
		recovery: "Ask the developer to provide the credentials, or retry with --offline. Offline adds a score penalty and is refused for prod-tier targets.",
	},
	"waiver_confirmation": {
		meaning:  "A waiver is written only after the developer confirms it. This is the question, not a failure.",
		recovery: "Ask needs_input.question (confirm or cancel). On confirm run needs_input.resume_with, which repeats the command with --confirmed. Never pass --confirmed unasked.",
	},

	"tracker_auth": {
		meaning:  "The tracker's credentials are missing, or it rejected them (HTTP 401 or 403).",
		recovery: "Halt. Tell the developer to check the tracker credentials — AI_FLOW_JIRA_EMAIL and AI_FLOW_JIRA_TOKEN for Jira, AI_FLOW_GITHUB_TOKEN (or GITHUB_TOKEN, GH_TOKEN) for GitHub Issues — and that the account may see the project or repository.",
	},
	"tracker_not_found": {
		meaning:  "The tracker answered 404: the issue, project or transition does not exist, the account cannot see it, or (GitHub Issues) the number is a pull request.",
		recovery: "Halt. Check the ticket key and the account's access to the project.",
	},
	"host_auth": {
		meaning:  "The git host rejected the token or denied the permission needed (HTTP 401 or 403 without rate-limit headers).",
		recovery: "Halt. Tell the developer to set AI_FLOW_GITHUB_TOKEN (or GITHUB_TOKEN, GH_TOKEN) with permission to push and open pull requests on the repository.",
	},
	"rate_limited": {
		meaning:  "The provider is still rate-limiting after 3 attempts with waits of up to 30 seconds, or asked for a longer wait. GitHub's 403 with rate-limit headers counts.",
		recovery: "Halt and retry later. Do not loop on the command.",
	},
	"live_auth": {
		meaning:  "Live infrastructure credentials were rejected. The spec's exit table documents it, but no command emits it today.",
		recovery: "Halt and name the credential or permission to fix.",
		reserved: true,
	},
	"tool_failed": {
		meaning:  "A provider or tool could not be reached or answered unusably (connection refused, DNS, a deadline, a cut-short or over-size body), or a tool failed without producing a report. error.message names which.",
		recovery: "Halt and name what failed. Check network, VPN and the tool's installation, then retry once.",
	},
	"pack_unreachable": {
		meaning:  "A standards pack source could not be fetched: the repository, the ref, the network or the credentials.",
		recovery: "Check standards.sources (repo and ref) in config.json and the network. A private source needs AI_FLOW_GITHUB_TOKEN, and `auth: true` when it is not on github.com. Then run `ai-flow standards sync`.",
	},
	"credentials_missing": {
		meaning:  "A credential the run was told to require is not set: `skills evals --require-credentials` without ANTHROPIC_API_KEY. The message names the variable.",
		recovery: "Set the variable the message names. In CI, make the secret available to this job.",
	},

	"invalid_result": {
		meaning:  "A file or result failed schema validation, or `standards check <dir>` found open must findings. error.details lists each problem.",
		recovery: "Fix the file or the findings that error.details names, then re-run.",
	},
	"unknown_report_format": {
		meaning:  "A tool report is in a format ai-flow cannot parse. The spec's exit table documents it, but no command emits it today.",
		recovery: "Show error.details and fix or replace the report.",
		reserved: true,
	},
	"pack_invalid": {
		meaning:  "A standards pack's manifest or contents break the pack schema or the loader's structural rules.",
		recovery: "Run `ai-flow standards validate DIR` on the pack and fix each violation in error.details.",
	},
	"check_runner_missing": {
		meaning:  "A pack check names a tool that has no runner on the execution allowlist, so the check can never run.",
		recovery: "Fix the check's tool in the pack, or upgrade ai-flow-cli to a release whose allowlist has it. Packs cannot extend the allowlist.",
	},
	"config_invalid": {
		meaning:  "config.json does not parse, or breaks a `config validate` rule.",
		recovery: "Run `ai-flow config validate` and fix each problem it reports.",
	},
}

// Explain returns code's explanation and reports whether code is a
// documented error.code.
func Explain(code string) (Explanation, bool) {
	ec, ok := codeExitCode[code]
	if !ok {
		return Explanation{}, false
	}
	t, ok := explanationTexts[code]
	if !ok {
		return Explanation{}, false
	}
	return Explanation{Code: code, ExitCode: ec, Meaning: t.meaning, Recovery: t.recovery, Reserved: t.reserved}, true
}

// Explanations returns every code's explanation, ordered by exit code
// and then by name.
func Explanations() []Explanation {
	out := make([]Explanation, 0, len(explanationTexts))
	for code := range explanationTexts {
		if e, ok := Explain(code); ok {
			out = append(out, e)
		}
	}
	slices.SortFunc(out, func(a, b Explanation) int {
		return cmp.Or(cmp.Compare(a.ExitCode, b.ExitCode), cmp.Compare(a.Code, b.Code))
	})
	return out
}
