package cli

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards/check"
)

// SARIF 2.1.0, only the parts `standards check <dir> --sarif` fills in.
// Field order is declaration order and the one map (partialFingerprints)
// is sorted by encoding/json, so the bytes are a function of the
// findings alone.
type (
	sarifLog struct {
		Schema  string     `json:"$schema"`
		Version string     `json:"version"`
		Runs    []sarifRun `json:"runs"`
	}
	sarifRun struct {
		Tool    sarifTool     `json:"tool"`
		Results []sarifResult `json:"results"`
	}
	sarifTool struct {
		Driver sarifDriver `json:"driver"`
	}
	sarifDriver struct {
		Name           string      `json:"name"`
		Version        string      `json:"version"`
		InformationURI string      `json:"informationUri"`
		Rules          []sarifRule `json:"rules"`
	}
	sarifRule struct {
		ID                   string             `json:"id"`
		ShortDescription     sarifText          `json:"shortDescription"`
		DefaultConfiguration sarifConfiguration `json:"defaultConfiguration"`
		Properties           sarifRuleProps     `json:"properties"`
	}
	sarifConfiguration struct {
		Level string `json:"level"`
	}
	sarifRuleProps struct {
		Kind  string `json:"kind"`
		Layer string `json:"layer"`
		Tool  string `json:"tool"`
	}
	sarifText struct {
		Text string `json:"text"`
	}
	sarifResult struct {
		RuleID              string             `json:"ruleId"`
		Level               string             `json:"level"`
		Message             sarifText          `json:"message"`
		Locations           []sarifLocation    `json:"locations"`
		PartialFingerprints map[string]string  `json:"partialFingerprints"`
		Suppressions        []sarifSuppression `json:"suppressions,omitempty"`
	}
	sarifLocation struct {
		PhysicalLocation sarifPhysical `json:"physicalLocation"`
	}
	sarifPhysical struct {
		ArtifactLocation sarifArtifact `json:"artifactLocation"`
		Region           *sarifRegion  `json:"region,omitempty"`
	}
	sarifArtifact struct {
		URI       string `json:"uri"`
		URIBaseID string `json:"uriBaseId"`
	}
	sarifRegion struct {
		StartLine int `json:"startLine"`
	}
	sarifSuppression struct {
		Kind string `json:"kind"`
	}
)

const sarifFingerprintKey = "aiFlowFinding/v1"

// sarifLevel maps a check's severity onto SARIF's result levels.
func sarifLevel(severity string) string {
	switch severity {
	case "must":
		return "error"
	case "should":
		return "warning"
	default:
		return "note"
	}
}

// sarifURI is a finding's location relative to the checked directory.
// A target-scoped finding's File is relative to its target (the policy
// and scan runners run inside the target), so it is joined with the
// target's path from "<repo>:<path>". A finding with no file points at
// its target, or at the repository root: code scanning requires one
// location per result.
func sarifURI(f check.Finding) string {
	base := "."
	if _, p, ok := strings.Cut(f.Target, ":"); ok && p != "" {
		base = p
	}
	if f.File == "" {
		return path.Clean(base)
	}
	return path.Clean(path.Join(base, strings.ReplaceAll(f.File, "\\", "/")))
}

// buildSARIF renders the directory form's findings as one SARIF 2.1.0
// run: one rule per reported check, one result per open or waived
// finding (fixed ones no longer exist in the tree), waived ones marked
// as externally suppressed. Results and rules are sorted, so the same
// findings in any order give the same bytes.
func buildSARIF(version string, findings []check.Finding) ([]byte, error) {
	results := make([]sarifResult, 0, len(findings))
	rules := map[string]sarifRule{}
	for _, f := range findings {
		if f.Status != "open" && f.Status != "waived" {
			continue
		}
		uri := sarifURI(f)
		sum := sha256.Sum256([]byte(f.CheckID + "\x00" + uri + "\x00" + f.Message))
		r := sarifResult{
			RuleID:  f.CheckID,
			Level:   sarifLevel(f.Severity),
			Message: sarifText{Text: f.Message},
			Locations: []sarifLocation{{PhysicalLocation: sarifPhysical{
				ArtifactLocation: sarifArtifact{URI: uri, URIBaseID: "%SRCROOT%"},
			}}},
			PartialFingerprints: map[string]string{sarifFingerprintKey: hex.EncodeToString(sum[:])},
		}
		if f.Line > 0 {
			r.Locations[0].PhysicalLocation.Region = &sarifRegion{StartLine: f.Line}
		}
		if f.Status == "waived" {
			r.Suppressions = []sarifSuppression{{Kind: "external"}}
		}
		results = append(results, r)
		rules[f.CheckID] = sarifRule{
			ID:                   f.CheckID,
			ShortDescription:     sarifText{Text: f.CheckID},
			DefaultConfiguration: sarifConfiguration{Level: sarifLevel(f.Severity)},
			Properties:           sarifRuleProps{Kind: f.Kind, Layer: f.Layer, Tool: f.Tool},
		}
	}

	slices.SortFunc(results, func(a, b sarifResult) int {
		la, lb := a.Locations[0].PhysicalLocation, b.Locations[0].PhysicalLocation
		return cmp.Or(
			cmp.Compare(la.ArtifactLocation.URI, lb.ArtifactLocation.URI),
			cmp.Compare(startLine(la), startLine(lb)),
			cmp.Compare(a.RuleID, b.RuleID),
			cmp.Compare(a.Message.Text, b.Message.Text),
		)
	})
	ruleList := make([]sarifRule, 0, len(rules))
	for _, id := range slices.Sorted(maps.Keys(rules)) {
		ruleList = append(ruleList, rules[id])
	}

	log := sarifLog{
		Schema:  "https://json.schemastore.org/sarif-2.1.0.json",
		Version: "2.1.0",
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "ai-flow",
				Version:        version,
				InformationURI: "https://github.com/pmi-ai-stack/ai-flow-cli",
				Rules:          ruleList,
			}},
			Results: results,
		}},
	}
	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// writeSARIF writes findings to path as SARIF, atomically.
func writeSARIF(path, version string, findings []check.Finding) error {
	data, err := buildSARIF(version, findings)
	if err != nil {
		return fmt.Errorf("cli: build SARIF: %w", err)
	}
	if _, err := handoff.WriteFile(filepath.Dir(path), filepath.Base(path), data); err != nil {
		return err
	}
	return nil
}

func startLine(p sarifPhysical) int {
	if p.Region == nil {
		return 0
	}
	return p.Region.StartLine
}
