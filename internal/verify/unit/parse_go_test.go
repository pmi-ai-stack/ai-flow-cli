package unit

import (
	"os"
	"testing"
)

func TestParseGoPass(t *testing.T) {
	data, err := os.ReadFile("testdata/go/pass.jsonl")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	r, err := parseGo(data)
	if err != nil {
		t.Fatalf("parseGo: %v", err)
	}
	if r.Total != 1 || r.Passed != 1 || r.Failed != 0 || len(r.Failures) != 0 {
		t.Errorf("parseGo(pass) = %+v, want total=1 passed=1 failed=0", r)
	}
}

func TestParseGoFail(t *testing.T) {
	data, err := os.ReadFile("testdata/go/fail.jsonl")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	r, err := parseGo(data)
	if err != nil {
		t.Fatalf("parseGo: %v", err)
	}
	if r.Total != 2 || r.Passed != 1 || r.Failed != 1 {
		t.Errorf("parseGo(fail) = %+v, want total=2 passed=1 failed=1", r)
	}
	if len(r.Failures) != 1 || r.Failures[0].Name != "TestSubtract" {
		t.Fatalf("Failures = %+v, want one entry named TestSubtract", r.Failures)
	}
	if r.Failures[0].Message == "" {
		t.Error("Failures[0].Message is empty, want the captured output")
	}
}

func TestParseGoMalformed(t *testing.T) {
	if _, err := parseGo([]byte("not json\n")); err == nil {
		t.Error("parseGo(malformed): want an error")
	}
}
