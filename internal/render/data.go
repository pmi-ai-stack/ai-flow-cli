package render

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

// Repository is one repo.json entry, trimmed to what Output/PRBlock
// need.
type Repository struct {
	Name      string
	LocalPath string
}

// MutationRepo is one mutation-results.json repositories[] entry,
// trimmed to what Output/PRBlock need.
type MutationRepo struct {
	Repo      string
	Tool      string
	Score     float64
	Threshold int
	Survivors int
	Killed    int
}

// Accepted mirrors mutation-results.json's accepted_below_threshold
// object.
type Accepted struct {
	By     string
	Reason string
}

// SeverityCount is one checks-results.json severity's status tally —
// the block's own table row.
type SeverityCount struct {
	Severity            string
	Open, Fixed, Waived int
}

// PackSummary is one standards.json sources[] entry, trimmed to what
// the block prints: id@ref (or id@embedded for the reference pack).
type PackSummary struct {
	ID, Ref string
}

// WaiverSummary is one standards.json waivers[] entry, trimmed to what
// the block prints.
type WaiverSummary struct {
	CheckID, Repo, Reason, Expires string
}

// BlastRadiusEntry is one impact-results.json blast_radius.entries[]
// item, trimmed to what the block prints.
type BlastRadiusEntry struct {
	Env, Scope, Kind string
	Count            int
}

// Data is everything Output and PRBlock read to render — a pure value,
// so both are pure functions of it (07-determinism.md).
type Data struct {
	Key          string
	Repositories []Repository
	Mutation     []MutationRepo
	Accepted     *Accepted

	// HasStandards is true once standards.json has been written for
	// this ticket (from the plan stage onward) — the phase-14 plan's
	// design decision 16: the checks/packs/waivers block is absent
	// entirely, not merely empty, before that, since there is nothing
	// yet to summarize rather than a summary of zero. A Data literal
	// built by hand (every existing render test) leaves this false,
	// which is exactly the "before plan" behaviour those tests already
	// expect.
	HasStandards bool
	Checks       []SeverityCount
	Packs        []PackSummary
	Waivers      []WaiverSummary

	// HasImpact is true once impact-results.json has been written for
	// this ticket (phase 23) — the same "absent entirely, not zeroed"
	// convention HasStandards already established: no targets at all
	// means no OPS section, not a score of zero.
	HasImpact   bool
	Score       int
	BlastRadius []BlastRadiusEntry

	// HasRollback is true once rollback-results.json has been written.
	HasRollback   bool
	NonRevertible []string
	WatchList     []string

	// HasInfraTest is true once infra-test-results.json has been
	// written (phase 24) — the same "absent entirely, not zeroed"
	// convention HasImpact/HasRollback already established.
	HasInfraTest bool
	InfraTest    []InfraTestSummary
}

// InfraTestSummary is one infra-test-results.json target/harness entry,
// the subset Output/PRBlock render.
type InfraTestSummary struct {
	Target  string
	Harness string
	Tool    string
	Total   int
	Passed  int
	Failed  int
}

// repoArtifactWire is the subset of repo.schema.json Load needs.
type repoArtifactWire struct {
	Repositories []struct {
		Name      string `json:"name"`
		LocalPath string `json:"local_path"`
	} `json:"repositories"`
}

// mutationResultsWire is the subset of mutation-results.schema.json
// Load needs.
type mutationResultsWire struct {
	Repositories []struct {
		Repo      string  `json:"repo"`
		Tool      string  `json:"tool"`
		Score     float64 `json:"score"`
		Threshold int     `json:"threshold"`
		Survivors int     `json:"survivors"`
		Killed    int     `json:"killed"`
	} `json:"repositories"`
	AcceptedBelowThreshold *struct {
		By     string `json:"by"`
		Reason string `json:"reason"`
	} `json:"accepted_below_threshold"`
}

// Load reads ticketDir's repo.json (required — output is unreachable
// before locate completes, so its absence means this was called out of
// order) and mutation-results.json (optional — absent whenever
// mutation was skipped, e.g. no app files changed).
func Load(ticketDir, key string) (Data, error) {
	d := Data{Key: key}

	repoData, err := handoff.ReadArtifact(filepath.Join(ticketDir, "repo.json"))
	if err != nil {
		return Data{}, err
	}
	if repoData == nil {
		return Data{}, &envelope.Error{Code: "missing_artifact", Message: "repo.json not found; run repo locate first"}
	}
	var repoWire repoArtifactWire
	if err := json.Unmarshal(repoData, &repoWire); err != nil {
		return Data{}, fmt.Errorf("render: decode repo.json: %w", err)
	}
	for _, r := range repoWire.Repositories {
		d.Repositories = append(d.Repositories, Repository{Name: r.Name, LocalPath: r.LocalPath})
	}

	mutation, accepted, err := LoadMutation(ticketDir)
	if err != nil {
		return Data{}, err
	}
	d.Mutation = mutation
	d.Accepted = accepted

	hasStandards, checks, packs, waivers, err := LoadStandards(ticketDir)
	if err != nil {
		return Data{}, err
	}
	d.HasStandards = hasStandards
	d.Checks = checks
	d.Packs = packs
	d.Waivers = waivers

	hasImpact, score, blastRadius, err := LoadImpact(ticketDir)
	if err != nil {
		return Data{}, err
	}
	d.HasImpact = hasImpact
	d.Score = score
	d.BlastRadius = blastRadius

	hasRollback, nonRevertible, watchList, err := LoadRollback(ticketDir)
	if err != nil {
		return Data{}, err
	}
	d.HasRollback = hasRollback
	d.NonRevertible = nonRevertible
	d.WatchList = watchList

	hasInfraTest, infraTest, err := LoadInfraTest(ticketDir)
	if err != nil {
		return Data{}, err
	}
	d.HasInfraTest = hasInfraTest
	d.InfraTest = infraTest

	return d, nil
}

// infraTestResultsWire is the subset of infra-test-results.schema.json
// Load needs.
type infraTestResultsWire struct {
	Targets []struct {
		Target  string `json:"target"`
		Harness string `json:"harness"`
		Tool    string `json:"tool"`
		Total   int    `json:"total"`
		Passed  int    `json:"passed"`
		Failed  int    `json:"failed"`
	} `json:"targets"`
}

// LoadInfraTest reads ticketDir's infra-test-results.json (optional —
// absent before the infra-test stage has run).
func LoadInfraTest(ticketDir string) (hasInfraTest bool, summaries []InfraTestSummary, err error) {
	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, "infra-test-results.json"))
	if err != nil {
		return false, nil, err
	}
	if data == nil {
		return false, nil, nil
	}
	var w infraTestResultsWire
	if err := json.Unmarshal(data, &w); err != nil {
		return false, nil, fmt.Errorf("render: decode infra-test-results.json: %w", err)
	}
	for _, t := range w.Targets {
		summaries = append(summaries, InfraTestSummary{Target: t.Target, Harness: t.Harness, Tool: t.Tool, Total: t.Total, Passed: t.Passed, Failed: t.Failed})
	}
	return true, summaries, nil
}

// impactResultsWire is the subset of impact-results.schema.json Load
// needs.
type impactResultsWire struct {
	Score       int `json:"score"`
	BlastRadius struct {
		Entries []struct {
			Env   string `json:"env"`
			Scope string `json:"scope"`
			Kind  string `json:"kind"`
			Count int    `json:"count"`
		} `json:"entries"`
	} `json:"blast_radius"`
}

// LoadImpact reads ticketDir's impact-results.json (optional — absent
// before the impact stage has run). Exported separately from Load for
// the same reason LoadStandards/LoadMutation are.
func LoadImpact(ticketDir string) (hasImpact bool, score int, blastRadius []BlastRadiusEntry, err error) {
	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, "impact-results.json"))
	if err != nil {
		return false, 0, nil, err
	}
	if data == nil {
		return false, 0, nil, nil
	}
	var w impactResultsWire
	if err := json.Unmarshal(data, &w); err != nil {
		return false, 0, nil, fmt.Errorf("render: decode impact-results.json: %w", err)
	}
	for _, e := range w.BlastRadius.Entries {
		blastRadius = append(blastRadius, BlastRadiusEntry{Env: e.Env, Scope: e.Scope, Kind: e.Kind, Count: e.Count})
	}
	return true, w.Score, blastRadius, nil
}

// rollbackResultsWire is the subset of rollback-results.schema.json
// Load needs.
type rollbackResultsWire struct {
	NonRevertible []string `json:"non_revertible"`
	WatchList     []string `json:"watch_list"`
}

// LoadRollback reads ticketDir's rollback-results.json (optional —
// absent before the rollback stage has run).
func LoadRollback(ticketDir string) (hasRollback bool, nonRevertible, watchList []string, err error) {
	data, err := handoff.ReadArtifact(filepath.Join(ticketDir, "rollback-results.json"))
	if err != nil {
		return false, nil, nil, err
	}
	if data == nil {
		return false, nil, nil, nil
	}
	var w rollbackResultsWire
	if err := json.Unmarshal(data, &w); err != nil {
		return false, nil, nil, fmt.Errorf("render: decode rollback-results.json: %w", err)
	}
	return true, w.NonRevertible, w.WatchList, nil
}

// standardsWire is the subset of standards.schema.json Load needs.
type standardsWire struct {
	Sources []struct {
		ID  string `json:"id"`
		Ref string `json:"ref"`
	} `json:"sources"`
	Waivers []struct {
		CheckID string `json:"check_id"`
		Repo    string `json:"repo"`
		Reason  string `json:"reason"`
		Expires string `json:"expires"`
	} `json:"waivers"`
}

// checksResultsWire is the subset of checks-results.schema.json Load
// needs.
type checksResultsWire struct {
	Findings []struct {
		Severity string `json:"severity"`
		Status   string `json:"status"`
	} `json:"findings"`
}

// severityOrder is the block's own display order — must first, since
// it is the one severity that blocks a stage, then should, then may.
var severityOrder = []string{"must", "should", "may"}

// LoadStandards reads ticketDir's standards.json (optional — absent
// before the plan stage resolves it) and checks-results.json
// (optional — absent before any check has run). Exported separately
// from Load for the same reason LoadMutation is: a caller that already
// has the pieces it needs in memory should not have to re-read them
// through a file.
func LoadStandards(ticketDir string) (hasStandards bool, checks []SeverityCount, packs []PackSummary, waivers []WaiverSummary, err error) {
	standardsData, err := handoff.ReadArtifact(filepath.Join(ticketDir, "standards.json"))
	if err != nil {
		return false, nil, nil, nil, err
	}
	if standardsData == nil {
		return false, nil, nil, nil, nil
	}

	var sw standardsWire
	if err := json.Unmarshal(standardsData, &sw); err != nil {
		return false, nil, nil, nil, fmt.Errorf("render: decode standards.json: %w", err)
	}
	for _, s := range sw.Sources {
		packs = append(packs, PackSummary{ID: s.ID, Ref: s.Ref})
	}
	for _, w := range sw.Waivers {
		waivers = append(waivers, WaiverSummary{CheckID: w.CheckID, Repo: w.Repo, Reason: w.Reason, Expires: w.Expires})
	}

	counts := map[string]*SeverityCount{}
	for _, sev := range severityOrder {
		counts[sev] = &SeverityCount{Severity: sev}
	}

	checksData, err := handoff.ReadArtifact(filepath.Join(ticketDir, "checks-results.json"))
	if err != nil {
		return false, nil, nil, nil, err
	}
	if checksData != nil {
		var cw checksResultsWire
		if err := json.Unmarshal(checksData, &cw); err != nil {
			return false, nil, nil, nil, fmt.Errorf("render: decode checks-results.json: %w", err)
		}
		for _, f := range cw.Findings {
			c, ok := counts[f.Severity]
			if !ok {
				continue
			}
			switch f.Status {
			case "open":
				c.Open++
			case "fixed":
				c.Fixed++
			case "waived":
				c.Waived++
			}
		}
	}
	for _, sev := range severityOrder {
		checks = append(checks, *counts[sev])
	}

	return true, checks, packs, waivers, nil
}

// LoadMutation reads ticketDir's mutation-results.json (optional —
// absent whenever mutation was skipped, e.g. no app files changed).
// Exported separately from Load so a caller that already has repo.json's
// repositories in memory (pr create, which needs them for its own
// git-push/PR loop regardless of rendering) can build a Data without
// re-reading and re-decoding a file it already has.
func LoadMutation(ticketDir string) ([]MutationRepo, *Accepted, error) {
	mutData, err := handoff.ReadArtifact(filepath.Join(ticketDir, "mutation-results.json"))
	if err != nil {
		return nil, nil, err
	}
	if mutData == nil {
		return nil, nil, nil
	}

	var mutWire mutationResultsWire
	if err := json.Unmarshal(mutData, &mutWire); err != nil {
		return nil, nil, fmt.Errorf("render: decode mutation-results.json: %w", err)
	}
	var mutation []MutationRepo
	for _, r := range mutWire.Repositories {
		mutation = append(mutation, MutationRepo{
			Repo: r.Repo, Tool: r.Tool, Score: r.Score,
			Threshold: r.Threshold, Survivors: r.Survivors, Killed: r.Killed,
		})
	}
	var accepted *Accepted
	if mutWire.AcceptedBelowThreshold != nil {
		accepted = &Accepted{By: mutWire.AcceptedBelowThreshold.By, Reason: mutWire.AcceptedBelowThreshold.Reason}
	}
	return mutation, accepted, nil
}
