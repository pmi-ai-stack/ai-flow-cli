package handoff

import (
	"strings"
	"testing"
)

func TestCatalogTicketScopedCommandsHaveKeyFirst(t *testing.T) {
	// Catalog does not model argv positions directly, but every
	// ticket-scoped command must be marked so — this is the property
	// skills lint (phase 10) will check invocations against.
	found := map[string]bool{}
	for _, c := range Catalog() {
		found[c.Name] = true
	}
	ticketScoped := []string{
		"status", "stage should-run", "stage start", "stage complete", "stage approve", "stage skip", "stage reset",
		"ticket fetch", "ticket update", "ticket show",
		"repo locate", "repo confirm", "repo scope",
		"git prepare", "git checkout-base", "git checkout-branch", "git diff", "git commit", "git push",
		"pr create",
		"detect languages", "detect runners", "detect mutation", "detect e2e", "detect frontend",
		"test run", "e2e run", "visual run", "visual judge",
		"mutation scope", "mutation run", "mutation summarize",
		"summary render",
	}
	for _, name := range ticketScoped {
		if !found[name] {
			t.Errorf("Catalog() is missing %q", name)
		}
	}
	for _, c := range Catalog() {
		wantScoped := false
		for _, n := range ticketScoped {
			if c.Name == n {
				wantScoped = true
			}
		}
		if c.TicketScoped != wantScoped {
			t.Errorf("Catalog entry %q: TicketScoped = %v, want %v", c.Name, c.TicketScoped, wantScoped)
		}
	}
}

func TestCatalogStageCommandsNameStagePositional(t *testing.T) {
	for _, c := range Catalog() {
		if strings.HasPrefix(c.Name, "stage ") {
			if len(c.Positional) != 1 || c.Positional[0] != "stage" {
				t.Errorf("Catalog entry %q: Positional = %v, want [\"stage\"]", c.Name, c.Positional)
			}
		}
	}
}

func TestCatalogReturnsACopy(t *testing.T) {
	a := Catalog()
	a[0].Name = "mutated"
	b := Catalog()
	if b[0].Name == "mutated" {
		t.Error("Catalog() did not return a fresh copy")
	}
}
