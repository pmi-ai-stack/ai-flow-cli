package jenkins

import "strings"

// normalizeText trims trailing whitespace per line and drops blank
// lines — the whitespace-normalized text compare Drift/DryRun both
// use in place of a structured parse (§1.1.14/§1.1.15/§1.2.2 of the
// phase-27 plan: three genuinely different job-config shapes share no
// common parseable structure this phase can honestly build without a
// real Jenkins server to verify against).
func normalizeText(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	for _, l := range lines {
		l = strings.TrimRight(l, " \t\r")
		if l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
