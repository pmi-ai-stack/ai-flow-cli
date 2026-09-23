package standards

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/handoff"
)

// WaiversFile names a repository's own waivers, which live inside the
// repository checkout (so they are committed and reviewed with it),
// never under ~/.ai-flow. The directory is layers.go's repoDotDir —
// the same one RepoPackDir is built from.
const WaiversFile = "waivers.json"

// waiversSchemaVersion is the waivers.json schema's own version, moved
// only when that file's shape changes — the same meaning
// config.json's own schema_version carries.
const waiversSchemaVersion = 1

// UnwaivableKind is the one check kind a waiver may never name. The
// spec states it flatly: "`secrets` checks cannot be waived." A
// committed credential is not a standards disagreement a repository
// gets to settle for itself.
const UnwaivableKind = "secrets"

// Waiver is one <repo>/.ai-flow/waivers.json entry.
type Waiver struct {
	CheckID string `json:"check_id"`
	Reason  string `json:"reason"`
	Author  string `json:"author"`
	Created string `json:"created"`
	Expires string `json:"expires"`

	// Repo is not stored in the file — it is filled in by LoadWaivers
	// from the file's own location, so a resolution spanning several
	// repositories can still say which one a waiver came from.
	Repo string `json:"-"`
}

// waiversFileShape is exactly what is written to disk — Waiver's Repo
// field is deliberately absent from it.
type waiversFileShape struct {
	SchemaVersion int      `json:"schema_version"`
	Waivers       []Waiver `json:"waivers"`
}

// WaiversPath returns repoLocalPath's own waivers file path.
func WaiversPath(repoLocalPath string) string {
	return filepath.Join(repoLocalPath, repoDotDir, WaiversFile)
}

// LoadWaivers reads one repository's waivers, tagging each with
// repoName. A repository with no waivers file has no waivers — the
// same "missing means not written yet, not an error" convention
// handoff.ReadArtifact established.
func LoadWaivers(repoName, repoLocalPath string) ([]Waiver, error) {
	path := WaiversPath(repoLocalPath)
	data, err := handoff.ReadArtifact(path)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}

	var f waiversFileShape
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, &envelope.Error{
			Code:    "invalid_result",
			Message: fmt.Sprintf("%s does not parse as JSON: %v", path, err),
		}
	}
	for i := range f.Waivers {
		f.Waivers[i].Repo = repoName
	}
	return f.Waivers, nil
}

// AddWaiver writes w into repoLocalPath's waivers file, replacing any
// existing waiver for the same check_id — one waiver per check, so a
// renewal never leaves the expired original behind to be read back
// alongside it. Entries are sorted by check_id, and the write goes
// through handoff.WriteArtifact, so the file is schema-validated and
// atomically renamed into place like every other artifact.
func AddWaiver(repoLocalPath string, w Waiver) error {
	existing, err := LoadWaivers("", repoLocalPath)
	if err != nil {
		return err
	}

	out := make([]Waiver, 0, len(existing)+1)
	for _, e := range existing {
		if e.CheckID != w.CheckID {
			out = append(out, e)
		}
	}
	out = append(out, w)
	slices.SortFunc(out, func(a, b Waiver) int { return strings.Compare(a.CheckID, b.CheckID) })

	path := WaiversPath(repoLocalPath)
	_, err = handoff.WriteArtifact(filepath.Dir(path), WaiversFile, "waivers", waiversFileShape{
		SchemaVersion: waiversSchemaVersion,
		Waivers:       out,
	})
	return err
}

// ParseExpiry accepts either a full RFC 3339 instant or a bare
// YYYY-MM-DD date, which means midnight UTC that day. Both spellings
// are natural for a developer typing `--expires`, and both are
// unambiguous.
func ParseExpiry(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%q is neither an RFC 3339 instant nor a YYYY-MM-DD date", s)
	}
	return t.UTC(), nil
}

// Expired reports whether w's expiry has passed at now. An expiry that
// does not parse counts as expired: an unreadable expiry must not
// silently become a permanent suppression, which is exactly what
// treating it as "not expired" would do.
func Expired(w Waiver, now time.Time) bool {
	exp, err := ParseExpiry(w.Expires)
	if err != nil {
		return true
	}
	return !now.Before(exp)
}
