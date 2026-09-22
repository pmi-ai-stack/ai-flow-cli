package e2e

import (
	"os"
	"slices"
	"testing"
)

func TestParsePlaywright(t *testing.T) {
	data, err := os.ReadFile("testdata/playwright/report.json")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	r, err := parsePlaywright(data)
	if err != nil {
		t.Fatalf("parsePlaywright: %v", err)
	}
	if r.Total != 3 || r.Passed != 1 || r.Failed != 1 {
		t.Errorf("parsePlaywright = %+v, want total=3 passed=1 failed=1", r)
	}
	wantNames := []string{
		"login > logs in with valid credentials",
		"login > password reset > sends a reset email",
		"login > rejects invalid credentials",
	}
	if !slices.Equal(r.CaseNames, wantNames) {
		t.Errorf("CaseNames = %v, want %v", r.CaseNames, wantNames)
	}
	if len(r.Failures) != 1 || r.Failures[0].Name != "login > rejects invalid credentials" {
		t.Fatalf("Failures = %+v", r.Failures)
	}
	if r.Failures[0].Message != "expected error banner to be visible" {
		t.Errorf("Failures[0].Message = %q", r.Failures[0].Message)
	}
}

func TestParsePlaywrightMalformed(t *testing.T) {
	if _, err := parsePlaywright([]byte("not json")); err == nil {
		t.Error("parsePlaywright(malformed): want an error")
	}
}

func TestParsePlaywrightEmpty(t *testing.T) {
	r, err := parsePlaywright([]byte(`{"suites":[]}`))
	if err != nil {
		t.Fatalf("parsePlaywright: %v", err)
	}
	if r.Total != 0 || len(r.CaseNames) != 0 {
		t.Errorf("parsePlaywright(empty) = %+v, want zero value", r)
	}
}
