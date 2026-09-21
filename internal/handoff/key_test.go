package handoff

import "testing"

func TestValidateKey(t *testing.T) {
	valid := []string{"AI-1", "PROJ-42", "PLAT-1000", "A1-1", "AB12-7"}
	invalid := []string{"", "ai-1", "AI", "AI-", "-1", "AI-1a", "AI_1", "1AI-1", "AI-01a"}

	for _, k := range valid {
		if err := ValidateKey(k); err != nil {
			t.Errorf("ValidateKey(%q) = %v, want nil", k, err)
		}
	}
	for _, k := range invalid {
		if err := ValidateKey(k); err == nil {
			t.Errorf("ValidateKey(%q) = nil, want an error", k)
		}
	}
}

// TestReservedNamesNeverMatchKeyPattern proves the two namespaces
// (ticket keys and reserved lowercase entries under ~/.ai-flow/)
// cannot collide, as the spec states.
func TestReservedNamesNeverMatchKeyPattern(t *testing.T) {
	for _, name := range ReservedNames {
		if KeyPattern.MatchString(name) {
			t.Errorf("reserved name %q matches KeyPattern; the two namespaces must never collide", name)
		}
	}
}

func TestIsReserved(t *testing.T) {
	if !IsReserved("config.json") {
		t.Error("config.json should be reserved")
	}
	if IsReserved("PROJ-1") {
		t.Error("a ticket key should never be reserved")
	}
}
