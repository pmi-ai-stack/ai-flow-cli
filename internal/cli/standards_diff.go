package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/standards"
)

// referencePackArg names the embedded reference pack on the command
// line. An existing directory of that name wins, the same precedence
// --pack gives a directory.
const referencePackArg = "reference"

func newStandardsDiffCmd(cio cmdIO) *cobra.Command {
	return &cobra.Command{
		Use:   "diff A B",
		Short: "Show what changes between two standards packs (a directory, <git-url>@<ref>, or \"reference\"): entries added, removed or changed, and every changed file",
		Args:  cobra.ExactArgs(2),
		RunE: wrap("standards diff", cio.stdout, cio.exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			a, closeA, err := openPackArg(cmd.Context(), args[0])
			defer closeA()
			if err != nil {
				return Outcome{}, err
			}
			b, closeB, err := openPackArg(cmd.Context(), args[1])
			defer closeB()
			if err != nil {
				return Outcome{}, err
			}
			d, err := standards.Diff(a, b)
			if err != nil {
				return Outcome{}, err
			}
			return Outcome{Result: d}, nil
		}),
	}
}

// openPackArg loads one side of a diff: an existing directory, the
// embedded reference pack, or <git-url>@<ref> synced into a temporary
// home (resolvePackFlag, as `standards check --pack` does). The
// returned func releases the pack and any temporary sync; it is always
// safe to call.
func openPackArg(ctx context.Context, arg string) (*standards.Pack, func(), error) {
	noop := func() {}
	if info, err := os.Stat(arg); err == nil && info.IsDir() {
		return openPackDir(arg, "workdir", "", "", noop)
	}
	if arg == referencePackArg {
		p, err := standards.EmbeddedPack()
		return p, noop, err
	}
	if at := strings.LastIndex(arg, "@"); at <= 0 || at == len(arg)-1 || strings.Contains(arg[at+1:], ":") {
		return nil, noop, &envelope.Error{
			Code:    "usage",
			Message: fmt.Sprintf("%q is not an existing directory, <git-url>@<ref>, or %q", arg, referencePackArg),
		}
	}
	loc, cleanup, err := resolvePackFlag(ctx, arg)
	if err != nil {
		return nil, cleanup, err
	}
	return openPackDir(loc.Dir, loc.Ref, loc.Repo, loc.Commit, cleanup)
}

// openPackDir opens the pack in dir under its own manifest id — the
// name a reviewer recognizes, not the directory or source it came
// from. cleanup runs after the pack is closed.
func openPackDir(dir, ref, repo, commit string, cleanup func()) (*standards.Pack, func(), error) {
	m, err := standards.Load(dir)
	if err != nil {
		return nil, cleanup, err
	}
	p, err := standards.OpenPack(standards.LayerOrg, m.ID, ref, repo, commit, dir)
	if err != nil {
		return nil, cleanup, err
	}
	return p, func() { _ = p.Close(); cleanup() }, nil
}
