package check

// CapSeverity maps a tool's own graded severity onto ai-flow's three
// levels, capped at the check entry's own declared severity — the
// spec's "critical|high → must, medium → should, low|info → may,
// capped at the check entry's severity" rule.
//
// Unused by every runner in this package: none of the eight kinds
// implemented through phase 14 (format, lint, dependency, secrets,
// layout, docs, commit, pr) reports a graded severity of its own —
// each is pass/fail per violation, so every Finding here carries
// exactly EffectiveCheck.Severity untouched. This function exists as
// the one place phase 25's scan/policy runners (checkov, trivy config,
// zizmor, kube-linter, kyverno apply) plug in the mapping rather than
// each re-deriving it inline — see the phase-14 plan's design
// decision 5.
func CapSeverity(toolSeverity, checkSeverity string) string {
	mapped := mapToolSeverity(toolSeverity)
	if severityRank(mapped) > severityRank(checkSeverity) {
		return checkSeverity
	}
	return mapped
}

func mapToolSeverity(s string) string {
	switch s {
	case "critical", "high":
		return "must"
	case "medium":
		return "should"
	case "low", "info":
		return "may"
	default:
		return "should"
	}
}

// severityRank orders must < should < may (lower is stricter) so
// "capped at" reads as "never looser than the check's own severity."
// internal/standards/budget.go defines its own severityRank over the
// same three cases — a deliberate, not accidental, near-duplicate: it
// is unexported to that package, and the two defaults differ on
// purpose (that one sorts an unknown severity last, since the pack
// schema already constrains the value and seeing one is a bug
// upstream; this one treats an unmapped *tool* severity as `should`,
// matching mapToolSeverity's own default for "not on the graded
// list").
func severityRank(s string) int {
	switch s {
	case "must":
		return 0
	case "should":
		return 1
	case "may":
		return 2
	default:
		return 1
	}
}
