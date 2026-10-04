package envelope

import (
	"slices"
	"strings"
	"testing"
)

// TestEveryCodeIsExplained is the enforcement: a code in the exit table
// without prose, or prose for a code that is not in the table, fails the
// build — the same rule TestExitCodeTable applies to exit codes.
func TestEveryCodeIsExplained(t *testing.T) {
	known := KnownCodes()
	for _, code := range known {
		e, ok := Explain(code)
		if !ok {
			t.Errorf("code %q has no explanation", code)
			continue
		}
		if e.Code != code {
			t.Errorf("explanation for %q names code %q", code, e.Code)
		}
		want, _ := ExitCodeFor(code)
		if e.ExitCode != want {
			t.Errorf("%s: explanation exit code %d, table says %d", code, e.ExitCode, want)
		}
		for name, v := range map[string]string{"Meaning": e.Meaning, "Recovery": e.Recovery} {
			if strings.TrimSpace(v) == "" {
				t.Errorf("%s: %s is empty", code, name)
			}
			if strings.ContainsAny(v, "\n\r") {
				t.Errorf("%s: %s has a line break; it is one paragraph", code, name)
			}
			if v != strings.TrimSpace(v) {
				t.Errorf("%s: %s has surrounding whitespace", code, name)
			}
		}
	}
	for _, e := range Explanations() {
		if !slices.Contains(known, e.Code) {
			t.Errorf("explanation for %q, which is not in the exit-code table", e.Code)
		}
	}
	if got := len(Explanations()); got != len(known) {
		t.Errorf("%d explanations for %d codes", got, len(known))
	}
}

func TestExplainUnknownCode(t *testing.T) {
	if _, ok := Explain("no_such_code"); ok {
		t.Error("Explain(no_such_code) reported ok")
	}
}

func TestExplanationsAreOrderedByExitCodeThenName(t *testing.T) {
	all := Explanations()
	for i := 1; i < len(all); i++ {
		a, b := all[i-1], all[i]
		if a.ExitCode > b.ExitCode || (a.ExitCode == b.ExitCode && a.Code >= b.Code) {
			t.Fatalf("order broken between %s (exit %d) and %s (exit %d)", a.Code, a.ExitCode, b.Code, b.ExitCode)
		}
	}
}

// Reserved codes are the ones documented in the spec's table that no
// command emits. Pinned so the claim is revisited when one starts to be.
func TestReservedCodes(t *testing.T) {
	var got []string
	for _, e := range Explanations() {
		if e.Reserved {
			got = append(got, e.Code)
		}
	}
	slices.Sort(got)
	if want := []string{"live_auth", "unknown_report_format"}; !slices.Equal(got, want) {
		t.Errorf("reserved codes = %v, want %v", got, want)
	}
}
