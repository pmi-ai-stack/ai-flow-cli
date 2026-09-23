package standards

import "testing"

// TestAppliesToMatching is the applies_to contract: every non-empty
// dimension must intersect, and an empty dimension matches everything.
func TestAppliesToMatching(t *testing.T) {
	languages := []string{"go", "typescript"}
	addons := []string{"kubectl"}

	tests := []struct {
		name  string
		a     AppliesTo
		stage string
		want  bool
	}{
		{"no narrowing applies everywhere", AppliesTo{}, "implement", true},
		{"matching language", AppliesTo{Languages: []string{"go"}}, "implement", true},
		{"one of several languages", AppliesTo{Languages: []string{"python", "typescript"}}, "implement", true},
		{"no matching language", AppliesTo{Languages: []string{"python"}}, "implement", false},
		{"matching addon", AppliesTo{Addons: []string{"kubectl", "helm"}}, "dry-run", true},
		{"no matching addon", AppliesTo{Addons: []string{"terraform"}}, "dry-run", false},
		{"matching stage", AppliesTo{Stages: []string{"implement", "review"}}, "implement", true},
		{"other stage", AppliesTo{Stages: []string{"ship"}}, "implement", false},
		{
			"every dimension must hold",
			AppliesTo{Languages: []string{"go"}, Addons: []string{"kubectl"}, Stages: []string{"implement"}},
			"implement", true,
		},
		{
			"one failing dimension is enough to exclude",
			AppliesTo{Languages: []string{"go"}, Addons: []string{"terraform"}, Stages: []string{"implement"}},
			"implement", false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Applies(tc.a, languages, addons, tc.stage); got != tc.want {
				t.Errorf("Applies(%+v, stage %q) = %v, want %v", tc.a, tc.stage, got, tc.want)
			}
		})
	}
}

// TestAppliesToAnyStageIgnoresTheStageDimension proves the
// check-filtering variant: a check's stage narrowing decides when it
// runs, not whether it is part of the resolution at all.
func TestAppliesToAnyStageIgnoresTheStageDimension(t *testing.T) {
	a := AppliesTo{Languages: []string{"go"}, Stages: []string{"ship"}}

	if Applies(a, []string{"go"}, nil, "implement") {
		t.Error("Applies matched a ship-only entry against implement")
	}
	if !AppliesToAnyStage(a, []string{"go"}, nil) {
		t.Error("AppliesToAnyStage rejected a ship-only entry; the stage dimension must be ignored")
	}
	if AppliesToAnyStage(a, []string{"python"}, nil) {
		t.Error("AppliesToAnyStage ignored the language dimension too")
	}
}

// TestAddonNarrowedEntriesDoNotApplyWithNoAddons pins the correct
// behaviour before phase 18 exists: no addon is active, so nothing
// narrowed to one applies. This is the right answer, not a stub.
func TestAddonNarrowedEntriesDoNotApplyWithNoAddons(t *testing.T) {
	a := AppliesTo{Addons: []string{"terraform", "kubectl"}}
	if Applies(a, []string{"go"}, nil, "implement") {
		t.Error("an addon-narrowed entry applied with no active addons")
	}
	if AppliesToAnyStage(a, []string{"go"}, []string{}) {
		t.Error("an addon-narrowed entry applied against an empty addon set")
	}
}
