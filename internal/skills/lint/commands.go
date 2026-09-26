package lint

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/skills"
)

// codeSpanPattern finds every inline single-backtick span in a body,
// regardless of whether it also happens to sit inside a triple-fenced
// block used only for checklist formatting — see the plan's design
// decision 4 for why this single regex is enough: a standalone ``` line
// can never match it (its three consecutive backticks never satisfy
// "backtick, one-or-more-non-backtick, backtick").
var codeSpanPattern = regexp.MustCompile("`([^`\n]+)`")

func codeSpans(body string) []string {
	matches := codeSpanPattern.FindAllStringSubmatch(body, -1)
	spans := make([]string, len(matches))
	for i, m := range matches {
		spans[i] = m[1]
	}
	return spans
}

var catalogByName = sync.OnceValue(func() map[string]handoff.CommandSpec {
	m := make(map[string]handoff.CommandSpec)
	for _, c := range handoff.Catalog() {
		m[c.Name] = c
	}
	return m
})

// checkBody runs rules 8-10: unknown-command, arg-order and the
// command-argument half of unknown-stage-id. checkNonPortableToken (rule
// 7) and checkHandoffFileTokens (the other half of rule 10, over
// bare-filename code spans) are separate files, but all three share this
// file's codeSpans helper.
func checkBody(s skills.Skill) []Finding {
	var findings []Finding
	for _, span := range codeSpans(s.Body) {
		if !strings.HasPrefix(span, "ai-flow ") {
			continue
		}
		findings = append(findings, checkInvocation(s, span)...)
	}
	findings = append(findings, checkHandoffFileTokens(s)...)
	findings = append(findings, checkNonPortableTokens(s)...)
	return findings
}

func checkInvocation(s skills.Skill, invocation string) []Finding {
	tokens, err := splitCommandLine(invocation)
	if err != nil || len(tokens) < 2 {
		return []Finding{{Rule: RuleUnknownCommand, Skill: s.Name, File: "SKILL.md", Message: fmt.Sprintf("unparseable invocation: %q", invocation)}}
	}
	// tokens[0] is "ai-flow" itself.
	fields := tokens[1:]

	catalog := catalogByName()
	var spec handoff.CommandSpec
	var nameLen int
	// Try the longest possible name first ("ops test detect" must win
	// over a hypothetical "ops test" entry), down to a single word —
	// the catalog has no two entries where one is a prefix of the
	// other today, but this order is correct even if it ever did. A
	// miss is just a cheap map lookup, so this needs no precomputed
	// upper bound: it already generalizes to a future catalog name of
	// any length.
	for n := len(fields); n >= 1; n-- {
		if cs, ok := catalog[strings.Join(fields[:n], " ")]; ok {
			spec, nameLen = cs, n
			break
		}
	}
	if nameLen == 0 {
		return []Finding{{Rule: RuleUnknownCommand, Skill: s.Name, File: "SKILL.md", Message: fmt.Sprintf("%q is not in the command catalog", invocation)}}
	}

	rest := fields[nameLen:]
	if spec.TicketScoped {
		if len(rest) == 0 || rest[0] != "KEY" {
			return []Finding{{Rule: RuleArgOrder, Skill: s.Name, File: "SKILL.md", Message: fmt.Sprintf("%q: the ticket key (\"KEY\") must be the first positional argument", invocation)}}
		}
		rest = rest[1:]

		// The "stage" positional's literal value, when this command's
		// own catalog entry names one, must be a real stage id — the
		// command-argument half of unknown-stage-id.
		if len(spec.Positional) > 0 && spec.Positional[0] == "stage" && len(rest) > 0 && !strings.HasPrefix(rest[0], "--") {
			if !handoff.IsStage(handoff.Stage(rest[0])) {
				return []Finding{{Rule: RuleUnknownStageID, Skill: s.Name, File: "SKILL.md", Message: fmt.Sprintf("%q: %q is not a stage in StageTable()", invocation, rest[0])}}
			}
		}
	}

	for _, tok := range rest {
		if !strings.HasPrefix(tok, "--") {
			continue
		}
		flag := strings.TrimPrefix(tok, "--")
		if !slices.Contains(spec.Flags, flag) {
			return []Finding{{Rule: RuleArgOrder, Skill: s.Name, File: "SKILL.md", Message: fmt.Sprintf("%q: --%s is not a recognized flag for %q", invocation, flag, spec.Name)}}
		}
	}
	return nil
}

// splitCommandLine tokenizes a shell-like invocation string, honoring
// double-quoted substrings (so `--reason "a mutation-below-threshold
// reason"` is one token, not several). It is deliberately minimal — no
// single-quote or backslash-escape handling — since every command this
// codebase's own catalog and the SKILL.md template document only ever
// need double quotes around a free-text flag value.
func splitCommandLine(s string) ([]string, error) {
	var tokens []string
	var cur strings.Builder
	inQuotes := false
	hasCur := false

	flush := func() {
		if hasCur {
			tokens = append(tokens, cur.String())
			cur.Reset()
			hasCur = false
		}
	}

	for _, r := range s {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			hasCur = true
		case r == ' ' && !inQuotes:
			flush()
		default:
			cur.WriteRune(r)
			hasCur = true
		}
	}
	flush()

	if inQuotes {
		return nil, fmt.Errorf("lint: unterminated quote in %q", s)
	}
	return tokens, nil
}
