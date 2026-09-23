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
		"standards resolve", "standards show", "standards waive", "standards check",
		"ops detect", "ops render", "ops drift", "ops dry-run",
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

func TestIsTicketScoped(t *testing.T) {
	for _, c := range Catalog() {
		if got := IsTicketScoped(c.Name); got != c.TicketScoped {
			t.Errorf("IsTicketScoped(%q) = %v, want %v (Catalog()'s own TicketScoped)", c.Name, got, c.TicketScoped)
		}
	}
	if IsTicketScoped("no-such-command") {
		t.Error("IsTicketScoped of an unknown name must be false, not an error")
	}
}

func TestSkipToolRunEvent(t *testing.T) {
	for _, c := range Catalog() {
		if got := SkipToolRunEvent(c.Name); got != c.SkipToolRunEvent {
			t.Errorf("SkipToolRunEvent(%q) = %v, want %v (Catalog()'s own SkipToolRunEvent)", c.Name, got, c.SkipToolRunEvent)
		}
		// Every "stage *" row must set it — the property this field
		// exists to make explicit, not just a naming convention.
		if strings.HasPrefix(c.Name, "stage ") && !c.SkipToolRunEvent {
			t.Errorf("Catalog entry %q: SkipToolRunEvent = false, want true for every stage command", c.Name)
		}
	}
	if SkipToolRunEvent("no-such-command") {
		t.Error("SkipToolRunEvent of an unknown name must be false, not an error")
	}
}
