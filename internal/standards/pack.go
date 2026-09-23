package standards

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/packs"
)

// Layer names one of the three precedence levels, least specific
// first. The zero value is deliberately not a layer: every Pack is
// built by a constructor that sets one.
const (
	LayerOrg  = "org"
	LayerTeam = "team"
	LayerRepo = "repo"
)

// EmbeddedRef and EmbeddedCommit are the ref/commit spelling for the
// pack compiled into this binary — the same "embedded" doctor's own
// synthetic pack row has used since phase 12, so doctor, standards.json
// and guidance.md never disagree about how to name it.
const (
	EmbeddedRef    = "embedded"
	EmbeddedCommit = "embedded"
)

// Pack is one loaded standards pack: its manifest's content plus the
// filesystem its sections and check configuration are read from.
//
// The spec's own Go interface sketch spells the last field
// `Root *os.Root`. This uses fs.FS instead, for one reason: the
// embedded reference pack is the org layer whenever
// config.standards.sources is empty, and it has no filesystem root at
// all — representing it as an *os.Root would mean materializing the
// embed into a temporary directory on every resolve. os.Root.FS()
// bridges the two, so a directory pack still gets os.Root's
// escape-confinement and both kinds of pack are read by one code path
// (see the phase-13 plan's design decision 3).
type Pack struct {
	ID      string
	Version string
	Layer   string
	Ref     string
	// Repo is the source's configured git URL or local path, empty for
	// the embedded pack and for a repository's own in-tree pack.
	Repo string
	// Commit is the git commit this pack was synced at, EmbeddedCommit
	// for the embedded pack, or empty for a pack with no commit at all
	// (a local-path source, a repository's own .ai-flow/standards/).
	Commit string
	// Fingerprint is sha256 over the pack tree's sorted
	// path→content-digest list — what actually moves the resolution
	// hash when a pack's content changes underneath a pinned ref.
	Fingerprint string

	Sections  []Section
	Checks    []Check
	Overrides []Override

	// FS reads this pack's own content — an os.Root's own FS for a
	// directory pack, the embed for the reference pack, and a
	// one-file in-memory FS for the synthetic repo.instructions pack
	// (see layers.go's instructionsPack). Always non-nil, so
	// ReadSection has exactly one code path.
	FS fs.FS

	// closer releases the *os.Root behind FS, for a pack opened from a
	// directory. Nil for the embedded pack.
	closer func() error
}

// Close releases whatever the pack holds open. Safe on a pack that
// holds nothing, and safe to call more than once.
func (p *Pack) Close() error {
	if p == nil || p.closer == nil {
		return nil
	}
	c := p.closer
	p.closer = nil
	return c()
}

// ClosePacks closes every pack in ps, discarding errors — the deferred
// shape every caller that loads a layer set wants. Read-only roots have
// nothing to flush, so a close failure is not information any caller
// can act on.
func ClosePacks(ps []*Pack) {
	for _, p := range ps {
		_ = p.Close()
	}
}

// OpenPack loads the pack in dir at the given layer. id/ref/repo/commit
// are the source's own identity, which the pack's directory cannot
// know: a synced pack's cache directory is named <id>@<ref> but its
// manifest carries the upstream author's id, and the caller is the one
// that knows which layer it is loading for.
func OpenPack(layer, id, ref, repo, commit, dir string) (*Pack, error) {
	m, err := Load(dir)
	if err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("standards: open %s: %w", dir, err)
	}

	fp, err := Fingerprint(root.FS())
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("standards: fingerprint %s: %w", dir, err)
	}

	return &Pack{
		ID: id, Version: m.Version, Layer: layer, Ref: ref, Repo: repo, Commit: commit,
		Fingerprint: fp,
		Sections:    m.Sections, Checks: m.Checks, Overrides: m.Overrides,
		FS: root.FS(), closer: root.Close,
	}, nil
}

// EmbeddedPack loads the reference pack compiled into this binary —
// the org layer whenever config.standards.sources is empty.
func EmbeddedPack() (*Pack, error) {
	sub, err := fs.Sub(packs.FS, "reference")
	if err != nil {
		// packs.FS always embeds reference/ — a failure here is a
		// broken embed, a programmer error, not a possible runtime
		// state (04-errors.md rule 9), exactly as Export already treats
		// the identical call.
		panic(fmt.Sprintf("standards: embedded packs.FS has no reference/: %v", err))
	}

	data, err := fs.ReadFile(sub, ManifestFile)
	if err != nil {
		panic(fmt.Sprintf("standards: embedded reference pack has no %s: %v", ManifestFile, err))
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		panic(fmt.Sprintf("standards: embedded reference pack's %s does not parse: %v", ManifestFile, err))
	}

	fp, err := Fingerprint(sub)
	if err != nil {
		return nil, fmt.Errorf("standards: fingerprint the embedded reference pack: %w", err)
	}

	return &Pack{
		ID: m.ID, Version: m.Version, Layer: LayerOrg, Ref: EmbeddedRef, Commit: EmbeddedCommit,
		Fingerprint: fp,
		Sections:    m.Sections, Checks: m.Checks, Overrides: m.Overrides,
		FS: sub,
	}, nil
}

// Fingerprint is sha256 over every file in fsys, as the sorted list of
// "<path>\x00<sha256 of content>\n" lines. Content-addressed rather
// than mtime- or size-based, so two packs with identical content
// fingerprint identically no matter how they got there — the property
// phase 12's "a tag, a commit and a local path produce identical
// trees" already established for the trees themselves.
func Fingerprint(fsys fs.FS) (string, error) {
	var lines []string
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		lines = append(lines, path+"\x00"+hex.EncodeToString(sum[:])+"\n")
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Strings(lines)
	h := sha256.New()
	for _, l := range lines {
		// hash.Hash.Write is documented never to return an error.
		h.Write([]byte(l))
	}
	return sha256Prefixed(h.Sum(nil)), nil
}

// sha256Prefixed renders a digest the way every hash this package
// writes into an artifact is spelled: "sha256:" then lowercase hex.
// One spelling, so a fingerprint, a resolution hash and a synthetic
// pack's own digest can never drift apart in how they look.
func sha256Prefixed(sum []byte) string {
	return "sha256:" + hex.EncodeToString(sum)
}

// oneFileFS is an fs.FS holding a single named file in memory — what
// the synthetic repo.instructions pack reads through, so Pack.FS is
// never nil and ReadSection needs no special case. Deliberately
// minimal: it serves exactly one Open of exactly one name, which is
// all fs.ReadFile needs.
type oneFileFS struct {
	name string
	data []byte
}

func (o oneFileFS) Open(name string) (fs.File, error) {
	if name != o.name {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &oneFile{Reader: bytes.NewReader(o.data), info: oneFileInfo{name: o.name, size: int64(len(o.data))}}, nil
}

type oneFile struct {
	*bytes.Reader
	info oneFileInfo
}

func (f *oneFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (f *oneFile) Close() error               { return nil }

type oneFileInfo struct {
	name string
	size int64
}

func (i oneFileInfo) Name() string       { return i.name }
func (i oneFileInfo) Size() int64        { return i.size }
func (i oneFileInfo) Mode() fs.FileMode  { return 0o400 }
func (i oneFileInfo) ModTime() time.Time { return time.Time{} }
func (i oneFileInfo) IsDir() bool        { return false }
func (i oneFileInfo) Sys() any           { return nil }

// OverrideReason returns the reason this pack declared for replacing
// id, and whether it declared one at all.
func (p *Pack) OverrideReason(id string) (string, bool) {
	for _, o := range p.Overrides {
		if o.ID == id {
			return o.Reason, true
		}
	}
	return "", false
}

// ReadSection returns the bytes of the guidance file s names, read
// through the pack's own FS — including for the synthetic
// repo.instructions pack, whose FS holds its one file in memory (see
// layers.go's instructionsPack).
func (p *Pack) ReadSection(s Section) ([]byte, error) {
	data, err := fs.ReadFile(p.FS, s.Path)
	if err != nil {
		return nil, fmt.Errorf("standards: pack %s: read section %s (%s): %w", p.ID, s.ID, s.Path, err)
	}
	return data, nil
}
