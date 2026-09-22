package unit

import (
	"os"
	"testing"
)

func TestParsePytestPass(t *testing.T) {
	data, err := os.ReadFile("testdata/pytest/pass.xml")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	r, err := parsePytest(data)
	if err != nil {
		t.Fatalf("parsePytest: %v", err)
	}
	if r.Total != 1 || r.Passed != 1 || r.Failed != 0 || len(r.Failures) != 0 {
		t.Errorf("parsePytest(pass) = %+v, want total=1 passed=1 failed=0", r)
	}
}

func TestParsePytestFail(t *testing.T) {
	data, err := os.ReadFile("testdata/pytest/fail.xml")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	r, err := parsePytest(data)
	if err != nil {
		t.Fatalf("parsePytest: %v", err)
	}
	if r.Total != 2 || r.Passed != 1 || r.Failed != 1 {
		t.Errorf("parsePytest(fail) = %+v, want total=2 passed=1 failed=1", r)
	}
	if len(r.Failures) != 1 || r.Failures[0].Name != "test_add::test_subtract" {
		t.Fatalf("Failures = %+v, want one entry named test_add::test_subtract", r.Failures)
	}
	if r.Failures[0].Message != "assert 0 == 1" {
		t.Errorf("Failures[0].Message = %q, want the failure message", r.Failures[0].Message)
	}
}

func TestParsePytestMalformed(t *testing.T) {
	if _, err := parsePytest([]byte("not xml")); err == nil {
		t.Error("parsePytest(malformed): want an error")
	}
}
