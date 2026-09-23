package standards

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

// ArtifactFile is the resolved artifact's name inside a ticket's
// handoff directory. It shares its filename with a *pack's* own
// manifest, which is a different thing at a different path
// (<pack-dir>/standards.json) — the collision is the spec's, and is
// called out here because it is a genuine readability hazard.
const ArtifactFile = "standards.json"

// artifactAppliesTo mirrors standards.schema.json's appliesTo $def.
type artifactAppliesTo struct {
	Addons    []string `json:"addons,omitempty"`
	Languages []string `json:"languages,omitempty"`
	Stages    []string `json:"stages,omitempty"`
}

// ArtifactSource is one sources[] entry.
type ArtifactSource struct {
	Commit      string `json:"commit,omitempty"`
	Fingerprint string `json:"fingerprint"`
	ID          string `json:"id"`
	Layer       string `json:"layer"`
	Ref         string `json:"ref"`
	Repo        string `json:"repo,omitempty"`
	Version     string `json:"version,omitempty"`
}

// ArtifactStageBudget is one budget.per_stage[] entry.
type ArtifactStageBudget struct {
	Dropped           []string `json:"dropped"`
	Kept              []string `json:"kept"`
	MustOverflowBytes int      `json:"must_overflow_bytes"`
	Stage             string   `json:"stage"`
}

// ArtifactBudget is the budget block.
type ArtifactBudget struct {
	PerStage []ArtifactStageBudget `json:"per_stage"`
	Tokens   int                   `json:"tokens"`
}

// ArtifactSection is one sections[] entry.
type ArtifactSection struct {
	AppliesTo      *artifactAppliesTo `json:"applies_to,omitempty"`
	Conflict       string             `json:"conflict,omitempty"`
	DroppedFrom    []string           `json:"dropped_from,omitempty"`
	ID             string             `json:"id"`
	Layer          string             `json:"layer"`
	OverrideReason string             `json:"override_reason,omitempty"`
	OverriddenBy   *string            `json:"overridden_by"`
	Path           string             `json:"path"`
	Severity       string             `json:"severity"`
}

// ArtifactWaiver is one waiver as the artifact records it.
type ArtifactWaiver struct {
	Author   string `json:"author"`
	CheckID  string `json:"check_id"`
	Created  string `json:"created"`
	Expired  bool   `json:"expired"`
	Expires  string `json:"expires"`
	Reason   string `json:"reason"`
	RepoName string `json:"repo,omitempty"`
}

// ArtifactCheck is one checks[] entry.
type ArtifactCheck struct {
	AppliesTo      *artifactAppliesTo `json:"applies_to,omitempty"`
	Conflict       string             `json:"conflict,omitempty"`
	Config         string             `json:"config,omitempty"`
	ID             string             `json:"id"`
	Input          string             `json:"input,omitempty"`
	Kind           string             `json:"kind"`
	Layer          string             `json:"layer"`
	OverrideReason string             `json:"override_reason,omitempty"`
	OverriddenBy   *string            `json:"overridden_by"`
	Severity       string             `json:"severity"`
	Tool           string             `json:"tool,omitempty"`
	Waived         bool               `json:"waived"`
	Waiver         *ArtifactWaiver    `json:"waiver,omitempty"`
}

// ArtifactConflict is one conflicts[] entry.
type ArtifactConflict struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Loser  string `json:"loser"`
	Rule   string `json:"rule"`
	Winner string `json:"winner"`
}

// Artifact mirrors standards.schema.json field for field.
type Artifact struct {
	Addons         []string           `json:"addons"`
	Budget         ArtifactBudget     `json:"budget"`
	Checks         []ArtifactCheck    `json:"checks"`
	Conflicts      []ArtifactConflict `json:"conflicts,omitempty"`
	Languages      []string           `json:"languages"`
	ResolutionHash string             `json:"resolution_hash"`
	Sections       []ArtifactSection  `json:"sections"`
	Sources        []ArtifactSource   `json:"sources"`
	Waivers        []ArtifactWaiver   `json:"waivers,omitempty"`
}

// ToArtifact projects a resolution onto the standards.json shape.
func ToArtifact(e *Effective) Artifact {
	a := Artifact{
		Addons:         e.Addons,
		Languages:      e.Languages,
		ResolutionHash: e.Hash,
		Budget:         ArtifactBudget{Tokens: e.BudgetTokens, PerStage: []ArtifactStageBudget{}},
		Checks:         []ArtifactCheck{},
		Sections:       []ArtifactSection{},
		Sources:        []ArtifactSource{},
	}

	for _, p := range e.Packs {
		a.Sources = append(a.Sources, ArtifactSource{
			Commit: p.Commit, Fingerprint: p.Fingerprint, ID: p.ID,
			Layer: p.Layer, Ref: p.Ref, Repo: p.Repo, Version: p.Version,
		})
	}

	for _, b := range e.Budget {
		a.Budget.PerStage = append(a.Budget.PerStage, ArtifactStageBudget{
			Dropped: b.Dropped, Kept: b.Kept, MustOverflowBytes: b.MustOverflowBytes, Stage: b.Stage,
		})
	}

	for _, s := range e.Sections {
		a.Sections = append(a.Sections, ArtifactSection{
			AppliesTo: toArtifactAppliesTo(s.Section.AppliesTo), Conflict: s.Conflict,
			DroppedFrom: nonEmpty(s.DroppedFrom), ID: s.ID, Layer: s.Layer,
			OverrideReason: s.OverrideReason, OverriddenBy: nullable(s.OverriddenBy),
			Path: s.Path, Severity: s.Severity,
		})
	}

	for _, c := range e.Checks {
		ac := ArtifactCheck{
			AppliesTo: toArtifactAppliesTo(c.Check.AppliesTo), Conflict: c.Conflict,
			Config: c.Config, ID: c.ID, Input: c.Input, Kind: c.Kind, Layer: c.Layer,
			OverrideReason: c.OverrideReason, OverriddenBy: nullable(c.OverriddenBy),
			Severity: c.Severity, Tool: c.Tool, Waived: c.Waived,
		}
		if c.Waiver != nil {
			w := toArtifactWaiver(*c.Waiver)
			ac.Waiver = &w
		}
		a.Checks = append(a.Checks, ac)
	}

	for _, c := range e.Conflicts {
		a.Conflicts = append(a.Conflicts, ArtifactConflict{
			ID: c.ID, Kind: c.Kind, Loser: c.Loser, Rule: c.Rule, Winner: c.Winner,
		})
	}

	for _, w := range e.Waivers {
		a.Waivers = append(a.Waivers, toArtifactWaiver(w))
	}

	return a
}

func toArtifactWaiver(w WaiverRecord) ArtifactWaiver {
	return ArtifactWaiver{
		Author: w.Author, CheckID: w.CheckID, Created: w.Created, Expired: w.Expired,
		Expires: w.Expires, Reason: w.Reason, RepoName: w.Repo,
	}
}

func toArtifactAppliesTo(a AppliesTo) *artifactAppliesTo {
	if len(a.Languages) == 0 && len(a.Addons) == 0 && len(a.Stages) == 0 {
		return nil
	}
	return &artifactAppliesTo{Addons: a.Addons, Languages: a.Languages, Stages: a.Stages}
}

// nullable renders an empty layer name as JSON null — the schema's own
// `["string", "null"]` for overridden_by, where null means "this entry
// is the effective one."
func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// nonEmpty drops an empty slice so it is omitted rather than written
// as `[]` — dropped_from is optional in the schema, and the common case
// (nothing dropped) should not add a line to every section.
func nonEmpty(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	return in
}

// WriteArtifact writes e's standards.json into ticketDir, schema
// validated and atomically renamed into place like every other
// machine-readable write.
func WriteArtifact(ticketDir string, e *Effective) (string, error) {
	return handoff.WriteArtifact(ticketDir, ArtifactFile, "standards", ToArtifact(e))
}

// ReadArtifact returns ticketDir's standards.json, or (nil, nil) when
// there is none yet — the one place this package's "what does a
// previous resolution look like on disk" policy lives, so every reader
// (the idempotency check, `stage start`'s refresh rule, doctor's budget
// block) shares it.
//
// A standards.json this binary cannot parse is reported as absent
// rather than as an error: the next resolve overwrites it anyway, and
// refusing to resolve because the *previous* resolution is unreadable
// would strand the ticket.
func ReadArtifact(ticketDir string) (*Artifact, error) {
	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, ArtifactFile))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	var a Artifact
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, nil //nolint:nilerr // an unreadable previous resolution is "absent", not a failure — see the doc comment
	}
	return &a, nil
}

// ReadArtifactHash returns the resolution hash recorded in ticketDir's
// standards.json, or "" when there is none yet — what `standards
// resolve`'s idempotency and `stage start`'s refresh rule both compare
// against.
func ReadArtifactHash(ticketDir string) (string, error) {
	a, err := ReadArtifact(ticketDir)
	if err != nil || a == nil {
		return "", err
	}
	return a.ResolutionHash, nil
}

// WriteGuidance renders and writes guidance.md for one stage.
func WriteGuidance(ticketDir, key, stage string, e *Effective) (string, error) {
	body, err := RenderGuidance(e, key, stage)
	if err != nil {
		return "", fmt.Errorf("standards: render %s: %w", GuidanceFile, err)
	}
	return handoff.WriteFile(ticketDir, GuidanceFile, body)
}
