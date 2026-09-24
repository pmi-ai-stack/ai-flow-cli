package render

import (
	"fmt"
	"strings"
)

func heading(level int, text string) string {
	return strings.Repeat("#", level) + " " + text + "\n\n"
}

// block builds the section set Output and PRBlock share, at headings
// level deep (2 for output.md's own top-level sections, 3 nested under
// PRBlock's one wrapping heading) — see the plan's design decision 1.
func block(d Data, level int) string {
	var b strings.Builder

	b.WriteString(heading(level, "Repositories"))
	if len(d.Repositories) == 0 {
		b.WriteString("(no repositories recorded)\n\n")
	} else {
		for _, r := range d.Repositories {
			if r.LocalPath != "" {
				fmt.Fprintf(&b, "- %s (`%s`)\n", r.Name, r.LocalPath)
			} else {
				fmt.Fprintf(&b, "- %s\n", r.Name)
			}
		}
		b.WriteString("\n")
	}

	b.WriteString(heading(level, "Checks, packs and waivers"))
	writeChecksSection(&b, d)

	b.WriteString(heading(level, "Mutation"))
	if len(d.Mutation) == 0 {
		b.WriteString("No mutation results recorded.\n\n")
	} else {
		b.WriteString("| Repository | Tool | Score | Threshold | Survivors | Killed |\n")
		b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
		for _, m := range d.Mutation {
			fmt.Fprintf(&b, "| %s | %s | %.1f | %d | %d | %d |\n", m.Repo, m.Tool, m.Score, m.Threshold, m.Survivors, m.Killed)
		}
		b.WriteString("\n")
		if d.Accepted != nil {
			fmt.Fprintf(&b, "Accepted below threshold by %s: %s\n\n", d.Accepted.By, d.Accepted.Reason)
		}
	}

	writeImpactAndRollbackSection(&b, d, level)
	writeInfraTestSection(&b, d, level)

	return strings.TrimRight(b.String(), "\n") + "\n"
}

// writeInfraTestSection renders infra-test-results.json's own
// per-target-harness totals (phase 24) — absent entirely, not the
// heading with an empty body, before infra-test-results.json exists
// for this ticket, the same convention writeChecksSection/
// writeImpactAndRollbackSection already established.
func writeInfraTestSection(b *strings.Builder, d Data, level int) {
	if !d.HasInfraTest {
		return
	}
	b.WriteString(heading(level, "Infra tests"))
	if len(d.InfraTest) == 0 {
		b.WriteString("(no harnesses run)\n\n")
		return
	}
	b.WriteString("| Target | Harness | Tool | Total | Passed | Failed |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, s := range d.InfraTest {
		fmt.Fprintf(b, "| %s | %s | %s | %d | %d | %d |\n", s.Target, s.Harness, s.Tool, s.Total, s.Passed, s.Failed)
	}
	b.WriteString("\n")
}

// writeImpactAndRollbackSection renders score, blast radius and the
// rollback summary — absent entirely (not the heading with an empty
// body) when neither impact-results.json nor rollback-results.json
// exists yet for this ticket, the same "absent, not zeroed" convention
// writeChecksSection already established for HasStandards.
func writeImpactAndRollbackSection(b *strings.Builder, d Data, level int) {
	if !d.HasImpact && !d.HasRollback {
		return
	}
	b.WriteString(heading(level, "Impact and rollback"))

	if d.HasImpact {
		fmt.Fprintf(b, "**Score:** %d/100\n\n", d.Score)
		if len(d.BlastRadius) == 0 {
			b.WriteString("Blast radius: (none)\n\n")
		} else {
			b.WriteString("Blast radius:\n\n")
			for _, e := range d.BlastRadius {
				fmt.Fprintf(b, "- %s / %s / %s: %d\n", e.Env, e.Scope, e.Kind, e.Count)
			}
			b.WriteString("\n")
		}
	}

	if d.HasRollback {
		if len(d.NonRevertible) == 0 {
			b.WriteString("Non-revertible: (none)\n\n")
		} else {
			b.WriteString("Non-revertible:\n\n")
			for _, addr := range d.NonRevertible {
				fmt.Fprintf(b, "- %s\n", addr)
			}
			b.WriteString("\n")
		}
		if len(d.WatchList) == 0 {
			b.WriteString("Watch list: (none)\n\n")
		} else {
			fmt.Fprintf(b, "Watch list: %s\n\n", strings.Join(d.WatchList, ", "))
		}
	}
}

// writeChecksSection renders the checks/packs/waivers section (the
// phase-14 plan's design decision 16): absent entirely — a sentence,
// not a table with every count at zero — until standards.json exists
// for this ticket (from the plan stage onward), since before that
// there is nothing yet to summarize rather than a summary of zero.
func writeChecksSection(b *strings.Builder, d Data) {
	if !d.HasStandards {
		b.WriteString("No standards resolution recorded yet.\n\n")
		return
	}

	b.WriteString("| Severity | Open | Fixed | Waived |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, c := range d.Checks {
		fmt.Fprintf(b, "| %s | %d | %d | %d |\n", c.Severity, c.Open, c.Fixed, c.Waived)
	}
	b.WriteString("\n")

	if len(d.Packs) == 0 {
		b.WriteString("Packs: (none resolved)\n\n")
	} else {
		names := make([]string, len(d.Packs))
		for i, p := range d.Packs {
			names[i] = p.ID + "@" + p.Ref
		}
		fmt.Fprintf(b, "Packs: %s\n\n", strings.Join(names, ", "))
	}

	if len(d.Waivers) == 0 {
		b.WriteString("No waivers recorded.\n\n")
	} else {
		for _, w := range d.Waivers {
			fmt.Fprintf(b, "- %s (%s): %s, until %s\n", w.CheckID, w.Repo, w.Reason, w.Expires)
		}
		b.WriteString("\n")
	}
}

// Output renders output.md's full content — repositories, checks
// (stubbed), mutation scores and any accepted_below_threshold reason,
// plus the plan link. `summary render` (the output stage's own
// CLI-only driver) is this file's one writer.
func Output(d Data) string {
	var b strings.Builder
	b.WriteString("# Summary\n\n")
	b.WriteString("**Plan:** [plan.md](plan.md)\n\n")
	b.WriteString(block(d, 2))
	return b.String()
}

// PRBlock renders the block `pr create` appends to the agent-authored
// pr-body.md — "pr-body.md plus the generated summary block" (the
// command catalog's own words). No plan link: plan.md lives under
// ~/.ai-flow/<KEY>/, never inside the git repository a hosted PR
// description is read from, so a relative link to it would be dead
// there.
func PRBlock(d Data) string {
	var b strings.Builder
	b.WriteString("## ai-flow summary\n\n")
	b.WriteString(block(d, 3))
	return b.String()
}
