package unit

import (
	"os"
	"testing"
)

func TestParseJestPass(t *testing.T) {
	data, err := os.ReadFile("testdata/jest/pass.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	r, err := parseJest(data)
	if err != nil {
		t.Fatalf("parseJest: %v", err)
	}
	if r.Total != 1 || r.Passed != 1 || r.Failed != 0 || len(r.Failures) != 0 {
		t.Errorf("parseJest(pass) = %+v, want total=1 passed=1 failed=0", r)
	}
}

func TestParseJestFail(t *testing.T) {
	data, err := os.ReadFile("testdata/jest/fail.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	r, err := parseJest(data)
	if err != nil {
		t.Fatalf("parseJest: %v", err)
	}
	if r.Total != 2 || r.Passed != 1 || r.Failed != 1 {
		t.Errorf("parseJest(fail) = %+v, want total=2 passed=1 failed=1", r)
	}
	if len(r.Failures) != 1 || r.Failures[0].Name != "subtracts numbers" {
		t.Fatalf("Failures = %+v, want one entry named 'subtracts numbers'", r.Failures)
	}
	if r.Failures[0].Message != "Error: expected 0 to be 1" {
		t.Errorf("Failures[0].Message = %q, want the failure message", r.Failures[0].Message)
	}
}

func TestParseJestMalformed(t *testing.T) {
	if _, err := parseJest([]byte("not json")); err == nil {
		t.Error("parseJest(malformed): want an error")
	}
}
