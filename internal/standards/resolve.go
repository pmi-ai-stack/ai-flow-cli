package standards

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/config"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/strutil"
)

// ConflictOverrideNotDeclared is the one resolve-time conflict this
// phase can produce: a more specific layer tried to replace a less
// specific `must` without declaring it in its own manifest's
// overrides[]. The less specific entry wins and this is reported — it
// is not an error, per the spec's own "otherwise the org entry wins and
// the conflict is reported at resolve time."
const ConflictOverrideNotDeclared = "override_not_declared"

// ResolveInput carries everything a resolution is a pure function of.
// Anything absent from here cannot move the resolution hash, which is
// the property "same inputs, same bytes" rests on.
type ResolveInput struct {
	Home string
	Cfg  *config.Config
	Team string
	// Repos is every in-scope repository, in repo.json order.
	Repos []RepoRef
	// Languages and Addons are the detected sets the applies_to
	// narrowing is matched against. Addons is empty until phase 18
	// wires `ops detect`, which is correct rather than provisional: no
	// addon is active, so no addon-narrowed entry applies.
	Languages []string
	Addons    []string
	// Waivers is every in-scope repository's waivers, already loaded.
	Waivers []Waiver
	// BudgetTokens is config.standards.guidance_budget_tokens; 0 means
	// unbudgeted.
	BudgetTokens int
	// Now evaluates waiver expiry; nil means time.Now.
	Now func() time.Time
	// OrgPack and InstructionsOptional are passed through to LoadLayers
	// (see LayerInput) — set only by `standards check <dir>`.
	OrgPack              *PackLocation
	InstructionsOptional bool
}

// EffectiveSection is one section of the resolution: every section
// from every layer appears, with the shadowed ones annotated.
type EffectiveSection struct {
	ID       string
	Layer    string
	Severity string
	Path     string
	Pack     *Pack
	Section  Section

	// OverriddenBy names the layer that replaced this entry, or is
	// empty when this entry is the effective one.
	OverriddenBy string
	// OverrideReason is the reason the winning manifest declared, when
	// it declared one.
	OverrideReason string
	// Conflict is ConflictOverrideNotDeclared on the entry that lost
	// because it tried to replace a less specific must silently.
	Conflict string
	// DroppedFrom names the stages the budget dropped this section
	// from. Always non-nil for an effective section.
	DroppedFrom []string
	// Bytes is the section body's length, the budget's own unit.
	Bytes int
	// body is the normalized bytes measureSections already read and
	// normalized to produce Bytes. Kept so RenderGuidance does not read
	// and re-normalize the same file a second time; Body falls back to
	// the pack when it is absent, so nothing depends on it being set.
	body []byte
}

// Body returns the section's rendered bytes — the exact bytes Bytes
// counted. It reuses what measureSections already read, and reads
// through the pack only for a section that was never measured (a
// shadowed entry, or an EffectiveSection built outside Resolve).
func (s EffectiveSection) Body() ([]byte, error) {
	if s.body != nil {
		return s.body, nil
	}
	raw, err := s.Pack.ReadSection(s.Section)
	if err != nil {
		return nil, err
	}
	return normalizeBody(raw), nil
}

// Effective reports whether this entry is the one that survived
// precedence.
func (s EffectiveSection) Effective() bool { return s.OverriddenBy == "" }

// EffectiveCheck is one check of the resolution, with the same
// shadowing annotations plus its waiver state.
type EffectiveCheck struct {
	ID       string
	Layer    string
	Kind     string
	Tool     string
	Config   string
	Severity string
	Input    string
	Pack     *Pack
	Check    Check

	OverriddenBy   string
	OverrideReason string
	Conflict       string

	// Waived is true only for a live waiver. An expired waiver re-fires
	// — it is still recorded on Waiver, with Expired set, so the
	// artifact can say why the check is back.
	Waived bool
	Waiver *WaiverRecord
}

func (c EffectiveCheck) Effective() bool { return c.OverriddenBy == "" }

// WaiverRecord is a Waiver as the resolution records it: the file's own
// fields plus the expiry verdict the resolution computed.
type WaiverRecord struct {
	Waiver
	Expired bool
}

// Conflict is one reported precedence conflict.
type Conflict struct {
	ID     string
	Kind   string // "section" or "check"
	Rule   string
	Winner string
	Loser  string
}

// StageBudget is one stage's budget outcome.
type StageBudget struct {
	Stage             string
	Kept              []string
	Dropped           []string
	MustOverflowBytes int
}

// Effective is one complete resolution. It is stage-independent: one
// resolution covers every stage, and each section records the stages
// the budget dropped it from. guidance.md is the per-stage rendering
// of this same object (see the phase-13 plan's design decision 1).
type Effective struct {
	Hash      string
	Packs     []*Pack
	Languages []string
	Addons    []string
	Sections  []EffectiveSection
	Checks    []EffectiveCheck
	Conflicts []Conflict
	Waivers   []WaiverRecord
	Budget    []StageBudget
	// BudgetTokens is the budget the resolution was computed against.
	BudgetTokens int
	// MissingInstructions names every repository without AGENTS.md or
	// CLAUDE.md — only ever non-empty when ResolveInput.
	// InstructionsOptional allowed the resolution to proceed anyway.
	// Not part of Hash: it is a fact about the checkout, already
	// reflected by the instructions pack's presence and fingerprint.
	MissingInstructions []string
}

// Resolve layers every pack, applies precedence and overrides, marks
// waivers, evaluates the per-stage token budget and computes the
// resolution hash.
//
// The returned Effective borrows the packs it was built from: the
// caller closes them with ClosePacks once it is done reading section
// content through them.
func Resolve(in ResolveInput) (*Effective, []*Pack, error) {
	packs, missing, err := LoadLayers(LayerInput{
		Home: in.Home, Cfg: in.Cfg, Team: in.Team, Repos: in.Repos,
		OrgPack: in.OrgPack, InstructionsOptional: in.InstructionsOptional,
	})
	if err != nil {
		return nil, nil, err
	}

	now := clockOrDefault(in.Now)()
	languages := normalized(in.Languages)
	addons := normalized(in.Addons)

	sections, checks, conflicts := applyPrecedence(packs, languages, addons)

	waivers := markWaivers(checks, in.Waivers, now)

	if err := measureSections(sections); err != nil {
		ClosePacks(packs)
		return nil, nil, err
	}
	budget := applyBudget(sections, in.BudgetTokens, languages, addons)

	e := &Effective{
		Packs:        packs,
		Languages:    languages,
		Addons:       addons,
		Sections:     sections,
		Checks:       checks,
		Conflicts:    conflicts,
		Waivers:      waivers,
		Budget:       budget,
		BudgetTokens: in.BudgetTokens,

		MissingInstructions: missing,
	}
	e.Hash = resolutionHash(e)
	return e, packs, nil
}

// applyPrecedence folds every pack's sections and checks into one set
// per id, most specific layer winning — except where a more specific
// layer tried to replace a less specific `must` without declaring the
// override, in which case the less specific entry wins and a conflict
// is reported.
//
// Every entry is kept, winner and loser alike: the shadowed ones carry
// OverriddenBy so standards.json can show what a repository's own pack
// actually changed, rather than silently presenting the survivors as
// if nothing had been replaced.
func applyPrecedence(packs []*Pack, languages, addons []string) ([]EffectiveSection, []EffectiveCheck, []Conflict) {
	var sections []EffectiveSection
	var checks []EffectiveCheck
	var conflicts []Conflict

	// Each map holds, per id, the index of the entry currently winning,
	// so a third layer is compared against the current winner rather
	// than against whichever entry happened to come last. The winner's
	// layer and severity are read back from the slice, so there is only
	// ever one copy of them.
	sectionWinner := map[string]int{}
	checkWinner := map[string]int{}

	for _, p := range packs {
		sections, conflicts = foldSections(p, languages, addons, sections, sectionWinner, conflicts)
		checks, conflicts = foldChecks(p, languages, addons, checks, checkWinner, conflicts)
	}

	// The two loops above are the same algorithm over two types, left
	// as two loops on purpose: collapsing them needs either an
	// interface with one implementation or a helper taking six accessor
	// closures, both of which cost more than the duplication saves
	// (19-generics.md, and the same call internal/agenthost/guard and
	// internal/skills/lint already made).
	slices.SortStableFunc(sections, func(a, b EffectiveSection) int {
		return precedenceOrder(a.Severity, a.Layer, a.ID, b.Severity, b.Layer, b.ID)
	})
	slices.SortStableFunc(checks, func(a, b EffectiveCheck) int {
		if c := cmp.Compare(layerRank(a.Layer), layerRank(b.Layer)); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	slices.SortStableFunc(conflicts, func(a, b Conflict) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.ID, b.ID))
	})

	return sections, checks, conflicts
}

// foldSections folds one pack's applicable sections into the
// accumulating set, resolving precedence against whatever is currently
// winning each id. It returns the grown slices — the caller owns them.
func foldSections(p *Pack, languages, addons []string, sections []EffectiveSection, winner map[string]int, conflicts []Conflict) ([]EffectiveSection, []Conflict) {
	for _, s := range p.Sections {
		if !AppliesToAnyStage(s.AppliesTo, languages, addons) {
			continue
		}
		sections = append(sections, EffectiveSection{
			ID: s.ID, Layer: p.Layer, Severity: s.Severity, Path: s.Path,
			Pack: p, Section: s, DroppedFrom: []string{},
		})
		idx := len(sections) - 1

		prevIdx, seen := winner[s.ID]
		if !seen {
			winner[s.ID] = idx
			continue
		}
		prev := sections[prevIdx]
		reason, declared := p.OverrideReason(s.ID)
		if decideOverride(layerRank(p.Layer), layerRank(prev.Layer), prev.Severity, declared) {
			sections[prevIdx].OverriddenBy = p.Layer
			sections[prevIdx].OverrideReason = reason
			winner[s.ID] = idx
			continue
		}
		sections[idx].OverriddenBy = prev.Layer
		if layerRank(p.Layer) < layerRank(prev.Layer) {
			sections[idx].Conflict = ConflictOverrideNotDeclared
			conflicts = append(conflicts, Conflict{
				ID: s.ID, Kind: "section", Rule: ConflictOverrideNotDeclared,
				Winner: prev.Layer, Loser: p.Layer,
			})
		}
	}
	return sections, conflicts
}

// foldChecks is foldSections for checks — see that function's comment,
// and applyPrecedence's own note on why these stay two functions.
func foldChecks(p *Pack, languages, addons []string, checks []EffectiveCheck, winner map[string]int, conflicts []Conflict) ([]EffectiveCheck, []Conflict) {
	for _, c := range p.Checks {
		if !AppliesToAnyStage(c.AppliesTo, languages, addons) {
			continue
		}
		checks = append(checks, EffectiveCheck{
			ID: c.ID, Layer: p.Layer, Kind: c.Kind, Tool: c.Tool, Config: c.Config,
			Severity: c.Severity, Input: c.Input, Pack: p, Check: c,
		})
		idx := len(checks) - 1

		prevIdx, seen := winner[c.ID]
		if !seen {
			winner[c.ID] = idx
			continue
		}
		prev := checks[prevIdx]
		reason, declared := p.OverrideReason(c.ID)
		if decideOverride(layerRank(p.Layer), layerRank(prev.Layer), prev.Severity, declared) {
			checks[prevIdx].OverriddenBy = p.Layer
			checks[prevIdx].OverrideReason = reason
			winner[c.ID] = idx
			continue
		}
		checks[idx].OverriddenBy = prev.Layer
		if layerRank(p.Layer) < layerRank(prev.Layer) {
			checks[idx].Conflict = ConflictOverrideNotDeclared
			conflicts = append(conflicts, Conflict{
				ID: c.ID, Kind: "check", Rule: ConflictOverrideNotDeclared,
				Winner: prev.Layer, Loser: p.Layer,
			})
		}
	}
	return checks, conflicts
}

// decideOverride answers "does the candidate replace the current
// winner?" for one id.
//
// A less specific candidate never replaces a more specific winner. A
// more specific one does — unless the winner is a `must` and the
// candidate's own manifest did not declare the override, which is the
// spec's "replacing an org must requires overrides[{id, reason}];
// otherwise the org entry wins." Replacing a `should` or a `may` needs
// no declaration.
//
// The declaration requirement is applied to *any* more-specific layer,
// not only to a repository's own pack: the spec illustrates it with the
// repository manifest because that is the case it describes, states no
// rule at all for team-over-org, and letting a team pack silently
// replace an org `must` would defeat the rule's evident purpose (see
// the phase-13 plan's design decision 4).
func decideOverride(candidateRank, currentRank int, currentSeverity string, declared bool) bool {
	if candidateRank >= currentRank {
		return false
	}
	if currentSeverity == "must" && !declared {
		return false
	}
	return true
}

// markWaivers marks every effective check that a live waiver names,
// and returns every waiver seen — expired ones included, with Expired
// set, since an expired waiver re-fires and the artifact should say so
// rather than silently omitting it.
//
// A `secrets` check is never marked, whatever the waivers file says: a
// file hand-edited past `standards waive`'s own refusal must not become
// an effective suppression.
func markWaivers(checks []EffectiveCheck, waivers []Waiver, now time.Time) []WaiverRecord {
	records := make([]WaiverRecord, 0, len(waivers))
	for _, w := range waivers {
		records = append(records, WaiverRecord{Waiver: w, Expired: Expired(w, now)})
	}
	slices.SortStableFunc(records, func(a, b WaiverRecord) int {
		if c := strings.Compare(a.CheckID, b.CheckID); c != 0 {
			return c
		}
		return strings.Compare(a.Repo, b.Repo)
	})

	for i := range checks {
		if !checks[i].Effective() || checks[i].Kind == UnwaivableKind {
			continue
		}
		for j := range records {
			if records[j].CheckID != checks[i].ID {
				continue
			}
			rec := records[j]
			checks[i].Waiver = &rec
			checks[i].Waived = !rec.Expired
			if checks[i].Waived {
				break // a live waiver wins over any expired one for the same id
			}
		}
	}
	return records
}

// measureSections fills each effective section's Bytes from the
// content it will contribute to guidance.md — the budget's own unit.
func measureSections(sections []EffectiveSection) error {
	for i := range sections {
		if !sections[i].Effective() {
			continue
		}
		raw, err := sections[i].Pack.ReadSection(sections[i].Section)
		if err != nil {
			return err
		}
		body := normalizeBody(raw)
		sections[i].body = body
		sections[i].Bytes = len(body)
	}
	return nil
}

// applyBudget runs the budget once per stage that has any applicable
// section, filling each section's DroppedFrom as it goes.
func applyBudget(sections []EffectiveSection, budgetTokens int, languages, addons []string) []StageBudget {
	var out []StageBudget

	byID := map[string]int{}
	for i := range sections {
		if sections[i].Effective() {
			byID[sections[i].ID] = i
		}
	}

	for _, stage := range handoff.StageNames() {
		var candidates []budgetCandidate
		for i := range sections {
			s := sections[i]
			if !s.Effective() || !Applies(s.Section.AppliesTo, languages, addons, stage) {
				continue
			}
			candidates = append(candidates, budgetCandidate{
				id: s.ID, layer: s.Layer, severity: s.Severity, bytes: s.Bytes,
			})
		}
		if len(candidates) == 0 {
			continue
		}

		sel := selectForStage(candidates, budgetTokens)
		for _, id := range sel.Dropped {
			i := byID[id]
			sections[i].DroppedFrom = append(sections[i].DroppedFrom, stage)
		}
		out = append(out, StageBudget{
			Stage: stage, Kept: sel.Kept, Dropped: sel.Dropped,
			MustOverflowBytes: sel.MustOverflowBytes,
		})
	}
	return out
}

// SectionsForStage returns the sections guidance.md renders for one
// stage, in budget order, and the ids the budget dropped.
func (e *Effective) SectionsForStage(stage string) (kept []EffectiveSection, dropped []string, mustOverflowBytes int) {
	var sb *StageBudget
	for i := range e.Budget {
		if e.Budget[i].Stage == stage {
			sb = &e.Budget[i]
			break
		}
	}
	if sb == nil {
		return nil, []string{}, 0
	}

	byID := map[string]EffectiveSection{}
	for _, s := range e.Sections {
		if s.Effective() {
			byID[s.ID] = s
		}
	}
	for _, id := range sb.Kept {
		if s, ok := byID[id]; ok {
			kept = append(kept, s)
		}
	}
	return kept, sb.Dropped, sb.MustOverflowBytes
}

// Section returns the effective section with this id.
func (e *Effective) Section(id string) (EffectiveSection, bool) {
	for _, s := range e.Sections {
		if s.ID == id && s.Effective() {
			return s, true
		}
	}
	return EffectiveSection{}, false
}

// Check returns the effective check with this id.
func (e *Effective) Check(id string) (EffectiveCheck, bool) {
	for _, c := range e.Checks {
		if c.ID == id && c.Effective() {
			return c, true
		}
	}
	return EffectiveCheck{}, false
}

// Warnings is what a caller surfaces on the envelope: every declared
// override (so an org `must` replacement is visible on every ticket
// that uses it — the phase-13 plan's design decision 14) and every
// undeclared one that lost.
func (e *Effective) Warnings() []string {
	var out []string
	for _, s := range e.Sections {
		if s.OverriddenBy != "" && s.OverrideReason != "" {
			out = append(out, "section "+s.ID+" ("+s.Layer+") overridden by "+s.OverriddenBy+": "+s.OverrideReason)
		}
	}
	for _, c := range e.Checks {
		if c.OverriddenBy != "" && c.OverrideReason != "" {
			out = append(out, "check "+c.ID+" ("+c.Layer+") overridden by "+c.OverriddenBy+": "+c.OverrideReason)
		}
	}
	for _, c := range e.Conflicts {
		out = append(out, c.Kind+" "+c.ID+": the "+c.Loser+" layer replaces a "+c.Winner+" `must` without an overrides[] entry; the "+c.Winner+" entry wins")
	}
	for _, b := range e.Budget {
		if b.MustOverflowBytes > 0 {
			out = append(out, "stage "+b.Stage+": budget exceeded by must sections by "+strconv.Itoa(b.MustOverflowBytes)+" bytes")
		}
	}
	slices.Sort(out)
	return out
}

// normalized returns a sorted, deduped, never-nil copy — the shape
// every set in a resolution is stored and hashed in. The never-nil
// coercion is this package's own: strutil.DedupSorted deliberately
// returns nil for an empty result and leaves that choice to callers.
func normalized(in []string) []string {
	if out := strutil.DedupSorted(in); out != nil {
		return out
	}
	return []string{}
}
