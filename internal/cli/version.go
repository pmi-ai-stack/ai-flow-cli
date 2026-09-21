package cli

import (
	"io"
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// versionResult is the result of `ai-flow version`.
type versionResult struct {
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	Date          string `json:"date"`
	GoVersion     string `json:"go_version"`
	SchemaVersion int    `json:"schema_version"`
}

func newVersionCmd(bi BuildInfo, stdout io.Writer, exitCode *int) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print CLI, Go and schema versions",
		Args:  cobra.NoArgs,
		RunE: wrap("version", stdout, exitCode, func(cmd *cobra.Command, args []string) (Outcome, error) {
			return Outcome{Result: buildVersionResult(bi, runtime.Version())}, nil
		}),
	}
}

// buildVersionResult fills in bi from debug.ReadBuildInfo() when the
// linker did not inject it (a `go build` or `go run` outside the
// release path), and pairs it with goVersion — a parameter, not a direct
// runtime.Version() call, so tests can pin it.
func buildVersionResult(bi BuildInfo, goVersion string) versionResult {
	if bi.Version == "" {
		bi.Version = "dev"
	}
	if bi.Commit == "" || bi.Date == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				switch s.Key {
				case "vcs.revision":
					if bi.Commit == "" {
						bi.Commit = s.Value
					}
				case "vcs.time":
					if bi.Date == "" {
						bi.Date = s.Value
					}
				}
			}
		}
	}
	if bi.Commit == "" {
		bi.Commit = "unknown"
	}
	if bi.Date == "" {
		bi.Date = "unknown"
	}
	return versionResult{
		Version:       bi.Version,
		Commit:        bi.Commit,
		Date:          bi.Date,
		GoVersion:     goVersion,
		SchemaVersion: SchemaVersion,
	}
}
