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

// Data is everything Output and PRBlock read to render — a pure value,
// so both are pure functions of it (07-determinism.md).
type Data struct {
	Key          string
	Repositories []Repository
	Mutation     []MutationRepo
	Accepted     *Accepted
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

	return d, nil
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
