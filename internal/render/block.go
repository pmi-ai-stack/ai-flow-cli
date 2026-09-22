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
	b.WriteString("Standards packs, checks and waivers are not implemented yet (see phases 12-14).\n\n")

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

	return strings.TrimRight(b.String(), "\n") + "\n"
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
