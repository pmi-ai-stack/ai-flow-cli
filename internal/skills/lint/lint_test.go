package lint

import (
	"path/filepath"
	"testing"
)

func TestLint_Valid(t *testing.T) {
	res, err := Lint(filepath.Join("..", "..", "..", "testdata", "skills", "valid"))
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if len(res.Findings) != 0 {
		t.Errorf("Lint(valid) findings = %+v, want none", res.Findings)
	}
	want := []string{"addon-example-tool", "develop", "run-mutation"}
	if len(res.Skills) != len(want) {
		t.Fatalf("Lint(valid) Skills = %v, want %v", res.Skills, want)
	}
	for i, w := range want {
		if res.Skills[i] != w {
			t.Errorf("Lint(valid) Skills[%d] = %s, want %s", i, res.Skills[i], w)
		}
	}
}
