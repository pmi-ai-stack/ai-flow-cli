package standards

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// hashPack is one pack's identity contribution to the resolution hash.
//
// The spec's own input list says "pack commits"; Commit carries that
// literally (recorded at sync time, "embedded" for the embedded pack).
// Fingerprint is carried alongside because a local-path pack and a
// repository's own in-tree pack have no commit at all, and because a
// pack edited in place underneath a pinned ref must still move the
// hash. Strictly stronger than the spec's list, never weaker.
type hashPack struct {
	ID          string `json:"id"`
	Layer       string `json:"layer"`
	Ref         string `json:"ref"`
	Commit      string `json:"commit"`
	Fingerprint string `json:"fingerprint"`
}

// hashWaiver is one waiver's contribution. Reason and author are
// deliberately excluded: rewording a waiver changes nothing about which
// checks fire, and a hash that moved on a typo fix would re-resolve
// every ticket for no reason.
type hashWaiver struct {
	CheckID string `json:"check_id"`
	Repo    string `json:"repo"`
	Expires string `json:"expires"`
}

// hashInput is exactly what the resolution hash is computed over —
// the spec's own "(languages, addons, pack commits, waivers, budget)".
// Nothing else may be added here without also being an input Resolve
// is a pure function of, or "same inputs, same bytes" stops holding.
type hashInput struct {
	Addons       []string     `json:"addons"`
	BudgetTokens int          `json:"budget_tokens"`
	Languages    []string     `json:"languages"`
	Packs        []hashPack   `json:"packs"`
	Waivers      []hashWaiver `json:"waivers"`
}

// resolutionHash computes e's resolution hash. Marshalling a struct
// with sorted-by-declaration fields and already-sorted slices gives a
// byte-stable pre-image without a hand-rolled canonicalizer.
func resolutionHash(e *Effective) string {
	in := hashInput{
		Addons:       e.Addons,
		BudgetTokens: e.BudgetTokens,
		Languages:    e.Languages,
		Packs:        make([]hashPack, 0, len(e.Packs)),
		Waivers:      make([]hashWaiver, 0, len(e.Waivers)),
	}
	for _, p := range e.Packs {
		in.Packs = append(in.Packs, hashPack{
			ID: p.ID, Layer: p.Layer, Ref: p.Ref, Commit: p.Commit, Fingerprint: p.Fingerprint,
		})
	}
	for _, w := range e.Waivers {
		in.Waivers = append(in.Waivers, hashWaiver{CheckID: w.CheckID, Repo: w.Repo, Expires: w.Expires})
	}

	data, err := json.Marshal(in)
	if err != nil {
		// hashInput holds only strings, ints and slices of those —
		// encoding/json cannot fail on it. A failure here would be this
		// function's own bug, not a runtime state (04-errors.md rule 9).
		panic(fmt.Sprintf("standards: marshal resolution hash input: %v", err))
	}
	sum := sha256.Sum256(data)
	return sha256Prefixed(sum[:])
}
