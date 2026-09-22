package schema

import "testing"

// TestSkillFrontmatterTightened exercises the phase-10 tightening of
// skill-frontmatter.schema.json beyond the generic valid/invalid pair
// TestValidFixtureAccepted/TestInvalidFixtureRejected already cover:
// additionalProperties:false, the license/compatibility const values,
// and disable-model-invocation's boolean-only value constraint. See
// internal/skills/lint's own doc comment for why lint itself does not
// call Validate at runtime — this schema is still the machine-readable
// contract other callers read, and needs its own direct coverage.
func TestSkillFrontmatterTightened(t *testing.T) {
	const (
		name          = `"name": "dry-run-change"`
		description   = `"description": "Runs the read-only dry-run. Loaded by the develop orchestrator at the dry-run stage; not for direct use."`
		license       = `"license": "MIT"`
		compatibility = `"compatibility": "Requires the ai-flow CLI on PATH and a git checkout of the in-scope repositories."`
	)

	tests := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{
			name:    "all required fields present, valid values",
			json:    `{` + name + `,` + description + `,` + license + `,` + compatibility + `}`,
			wantErr: false,
		},
		{
			name:    "disable-model-invocation true is allowed by the schema",
			json:    `{` + name + `,` + description + `,` + license + `,` + compatibility + `,"disable-model-invocation":true}`,
			wantErr: false,
		},
		{
			name:    "unknown field rejected",
			json:    `{` + name + `,` + description + `,` + license + `,` + compatibility + `,"allowed-tools":["Read"]}`,
			wantErr: true,
		},
		{
			name:    "license other than MIT rejected",
			json:    `{` + name + `,` + description + `,"license":"Apache-2.0",` + compatibility + `}`,
			wantErr: true,
		},
		{
			name:    "compatibility other than the fixed sentence rejected",
			json:    `{` + name + `,` + description + `,` + license + `,"compatibility":"Works everywhere."}`,
			wantErr: true,
		},
		{
			name:    "disable-model-invocation false rejected (const true)",
			json:    `{` + name + `,` + description + `,` + license + `,` + compatibility + `,"disable-model-invocation":false}`,
			wantErr: true,
		},
		{
			name:    "missing license rejected",
			json:    `{` + name + `,` + description + `,` + compatibility + `}`,
			wantErr: true,
		},
		{
			name:    "missing compatibility rejected",
			json:    `{` + name + `,` + description + `,` + license + `}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate("skill-frontmatter", []byte(tt.json))
			if tt.wantErr && err == nil {
				t.Errorf("Validate(%s) = nil, want an error", tt.json)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate(%s) = %v, want nil", tt.json, err)
			}
		})
	}
}
