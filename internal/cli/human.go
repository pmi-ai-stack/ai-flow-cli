package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards/check"
)

// printHuman is --human's renderer. Three commands — the ones a person
// runs by hand — have a readable layout; every other command, and every
// failure, keeps envelope.PrintHuman's generic rendering exactly. The
// output goes through the same redaction funnel as the JSON envelope.
func printHuman(w io.Writer, e envelope.Envelope) error {
	if body, ok := humanBody(e); ok {
		if _, err := fmt.Fprintf(w, "%s: ok\n%s", e.Command, body); err != nil {
			return err
		}
		for _, warn := range e.Warnings {
			if _, err := fmt.Fprintf(w, "! %s\n", warn); err != nil {
				return err
			}
		}
		return nil
	}
	return envelope.PrintHuman(w, e)
}

// humanBody renders a successful envelope's result when its command has a
// layout and the result is the type that layout expects.
func humanBody(e envelope.Envelope) (string, bool) {
	if !e.OK {
		return "", false
	}
	switch r := e.Result.(type) {
	case doctorResult:
		if e.Command == "doctor" {
			return humanDoctor(r), true
		}
	case standardsCheckResult:
		if e.Command == "standards check" {
			return humanStandardsCheck(r), true
		}
	case statusResult:
		if e.Command == "status" {
			return humanStatus(r), true
		}
	}
	return "", false
}

type humanRows struct{ b strings.Builder }

// row writes "label  value" with the label padded to a fixed column.
func (h *humanRows) row(label, format string, args ...any) {
	fmt.Fprintf(&h.b, "%-15s%s\n", label, fmt.Sprintf(format, args...))
}

// more writes a continuation line under the previous row.
func (h *humanRows) more(format string, args ...any) {
	fmt.Fprintf(&h.b, "%-15s%s\n", "", fmt.Sprintf(format, args...))
}

func yesNo(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}

func humanReach(creds bool, reachable *bool, errMsg string) string {
	switch {
	case !creds:
		return "credentials not set"
	case reachable == nil:
		return "credentials set, not probed"
	case *reachable:
		return "credentials set, reachable"
	default:
		return "credentials set, UNREACHABLE: " + errMsg
	}
}

func toolSummary(tools []doctorTool) string {
	var found, missing []string
	for _, t := range tools {
		if t.Found {
			found = append(found, t.Name)
		} else {
			missing = append(missing, t.Name)
		}
	}
	return joinNonEmpty("; ", labelled("found", found), labelled("missing", missing))
}

func labelled(label string, items []string) string {
	if len(items) == 0 {
		return ""
	}
	return label + ": " + strings.Join(items, ", ")
}

func joinNonEmpty(sep string, parts ...string) string {
	var keep []string
	for _, p := range parts {
		if p != "" {
			keep = append(keep, p)
		}
	}
	if len(keep) == 0 {
		return "none"
	}
	return strings.Join(keep, sep)
}

func humanDoctor(r doctorResult) string {
	var h humanRows

	switch {
	case !r.Config.Exists:
		h.row("config", "not found: %s", r.Config.Path)
	case !r.Config.Valid:
		h.row("config", "INVALID: %s", r.Config.Path)
		for _, e := range r.Config.Errors {
			h.more("- %s", e)
		}
	default:
		h.row("config", "valid  %s", r.Config.Path)
	}

	var set, unset []string
	for _, v := range r.EnvVars {
		if v.Set {
			set = append(set, v.Name)
		} else {
			unset = append(unset, v.Name)
		}
	}
	h.row("env vars", "%s", joinNonEmpty("; ", labelled("set", set), labelled("not set", unset)))

	h.row("tracker", "%s", humanReach(r.Tracker.CredsSet, r.Tracker.Reachable, r.Tracker.Error))
	if len(r.Tracker.MissingStatuses) > 0 {
		h.more("statuses not in the tracker's workflow: %s", strings.Join(r.Tracker.MissingStatuses, ", "))
	}
	h.row("host", "%s", humanReach(r.Host.CredsSet, r.Host.Reachable, r.Host.Error))

	h.row("git", "%s", yesNo(r.Git.Found, "found  "+r.Git.Path, "NOT FOUND"))
	h.row("test runners", "%s", toolSummary(r.TestRunners))
	h.row("mutation tools", "%s", toolSummary(r.MutationTools))
	h.row("addon tools", "%s", toolSummary(r.AddonBinaries))

	if r.Active == nil {
		h.row("active ticket", "none")
	} else {
		note := ""
		if r.Active.Stale {
			note += "  (stale: idle past the guard threshold)"
		}
		if r.Active.DifferentCLIVersion {
			note += "  (written by a different CLI version)"
		}
		h.row("active ticket", "%s, stage %s%s", r.Active.Key, r.Active.Stage, note)
	}

	for i, host := range r.Hosts {
		label := ""
		if i == 0 {
			label = "agent hosts"
		}
		h.row(label, "%s: %s", host.Host, yesNo(host.Detected, "detected", "not detected")+errSuffix(host.Error))
	}
	for _, s := range r.HookSelftest {
		h.row("", "hook selftest %s: %s", s.Host, yesNo(s.Passed, "passed", "FAILED"))
	}

	if len(r.Packs) == 0 {
		h.row("packs", "none configured")
	}
	for i, p := range r.Packs {
		label := ""
		if i == 0 {
			label = "packs"
		}
		state := yesNo(p.Synced, "synced", "not synced")
		if p.Stale {
			state += ", stale"
		}
		if p.Valid != nil && !*p.Valid {
			state += ", INVALID"
		}
		h.row(label, "%s@%s (%s): %s%s", p.ID, p.Ref, p.Layer, state, errSuffix(p.Error))
	}

	if len(r.ExpiringWaivers) == 0 {
		h.row("waivers", "none expired or expiring")
	}
	for i, w := range r.ExpiringWaivers {
		label := ""
		if i == 0 {
			label = "waivers"
		}
		h.row(label, "%s: %s %s (%s)", w.Repo, w.CheckID, w.State, w.Expires)
	}

	h.row("render cache", "%d entries, %d stale", r.RenderCache.Entries, r.RenderCache.StaleEntries)
	return h.b.String()
}

func errSuffix(e string) string {
	if e == "" {
		return ""
	}
	return " - " + e
}

func humanStandardsCheck(r standardsCheckResult) string {
	var b strings.Builder
	subject := "dir " + r.Dir
	if r.Key != "" {
		subject = "ticket " + r.Key
	}
	var must, should, may int
	for _, f := range r.Findings {
		if f.Status != "open" {
			continue
		}
		switch f.Severity {
		case "must":
			must++
		case "should":
			should++
		default:
			may++
		}
	}
	fmt.Fprintf(&b, "%s, stage %s: %d open (%d must, %d should, %d may), %d fixed, %d waived\n",
		subject, r.Stage, r.Open, must, should, may, r.Fixed, r.Waived)

	for _, f := range r.Findings {
		if f.Status != "open" {
			continue
		}
		fmt.Fprintf(&b, "\n%-6s %s%s\n", strings.ToUpper(f.Severity), f.CheckID, findingWhere(f))
		fmt.Fprintf(&b, "       %s\n", f.Message)
		if f.FixHint != "" {
			fmt.Fprintf(&b, "       fix: %s\n", f.FixHint)
		}
	}
	for _, status := range []string{"fixed", "waived"} {
		var items []string
		for _, f := range r.Findings {
			if f.Status == status {
				items = append(items, strings.TrimSpace(f.CheckID+findingWhere(f)))
			}
		}
		if len(items) > 0 {
			fmt.Fprintf(&b, "\n%s: %s\n", status, strings.Join(items, ", "))
		}
	}
	if len(r.Skipped) > 0 {
		b.WriteString("\nnot run:\n")
		for _, s := range r.Skipped {
			fmt.Fprintf(&b, "  %s (%s): %s\n", s.CheckID, s.Target, s.Reason)
		}
	}
	return b.String()
}

// findingWhere is "  file:line" (two leading spaces), or "" with no file.
func findingWhere(f check.Finding) string {
	switch {
	case f.File == "":
		return ""
	case f.Line > 0:
		return fmt.Sprintf("  %s:%d", f.File, f.Line)
	default:
		return "  " + f.File
	}
}

func humanStatus(r statusResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s\n\n", r.Key, r.HandoffPath)
	for _, s := range r.Stages {
		line := fmt.Sprintf("  %-14s%s", s.Stage, s.State)
		if s.Approval != nil {
			line += "  approved by " + s.Approval.By
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	if r.NextStage == nil {
		b.WriteString("next: nothing, every stage is done\n")
		return b.String()
	}
	fmt.Fprintf(&b, "next: %s", *r.NextStage)
	if r.NextSkill != nil {
		fmt.Fprintf(&b, " -> skill %s", *r.NextSkill)
	}
	if r.NextState != nil {
		fmt.Fprintf(&b, " (%s)", *r.NextState)
	}
	b.WriteString("\n")
	if r.NextCommand != nil {
		fmt.Fprintf(&b, "run:  %s\n", *r.NextCommand)
	}
	return b.String()
}
