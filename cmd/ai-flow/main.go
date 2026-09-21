// Command ai-flow is the sole deterministic actor in the ai-flow
// pipeline: agent skills carry judgement, this binary carries every side
// effect (tracker, git, git host, infra tools, test and mutation
// runners). Every command prints exactly one JSON envelope on stdout;
// see package envelope for that contract and package cli for the
// commands that use it.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/cli"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

// version, commit and date are injected at link time via -X (see
// .goreleaser.yaml); debug.ReadBuildInfo() is the fallback when they are
// not, which is every `go build`/`go run` outside the release path.
var (
	version string
	commit  string
	date    string
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	exitCode := 0
	root := cli.NewRoot(cli.BuildInfo{Version: version, Commit: commit, Date: date}, os.Stdout, os.Stderr, &exitCode)

	if err := root.ExecuteContext(ctx); err != nil {
		// A cobra-level failure (bad flags, unknown command) happens
		// before any RunE runs, so no envelope has been printed and
		// exitCode is still its zero value. The contract still holds:
		// exactly one envelope on stdout, usage (2) for this case.
		if exitCode == 0 {
			exitCode = int(envelope.ExitUsage)
			_ = envelope.Print(os.Stdout, envelope.Fail("ai-flow", &envelope.Error{Code: "usage", Message: err.Error()}))
		}
	}

	os.Exit(exitCode)
}
