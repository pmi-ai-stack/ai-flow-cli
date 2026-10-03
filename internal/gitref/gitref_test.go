package gitref

import "testing"

func TestValid(t *testing.T) {
	for _, ok := range []string{"main", "master", "release/2026.10", "feature/PROJ-1", "bugfix/PROJ-12", "v1.2.3", "a-b_c"} {
		if err := Valid(ok); err != nil {
			t.Errorf("Valid(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{
		"", "-x", "--upload-pack=touch /tmp/x", "main branch", "a..b", "a@{1}", "a//b", "/main", "main/",
		".hidden", "main.", "main.lock", "a~1", "a^", "a:b", "a?", "a*", "a[", `a\b`, "a\tb", "a\x7f",
	} {
		if err := Valid(bad); err == nil {
			t.Errorf("Valid(%q) = nil, want an error", bad)
		}
	}
}
