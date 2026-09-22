// Package pitest implements mutation.Tool for PIT (pitest), the Java
// mutation-testing tool detect.Mutation names for any repository with
// a pom.xml (internal/detect/mutation.go).
package pitest

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/mutation"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/strutil"
)

// reportGlob is where PIT's own mutationCoverage goal writes its XML
// report by default: one timestamped subdirectory of
// target/pit-reports per run, each holding mutations.xml.
const reportsDir = "target/pit-reports"

// Tool implements mutation.Tool for pitest.
type Tool struct{}

var _ mutation.Tool = Tool{}

// fqnFor maps a Maven-standard-layout source file
// (".../src/main/java/<pkg>/<Name>.java") to its fully-qualified class
// name; ok is false when file has no src/main/java segment — the
// plan's design decision 13, an accepted gap rather than a guess.
func fqnFor(file string) (fqn string, ok bool) {
	const marker = "src/main/java/"
	idx := strings.Index(file, marker)
	if idx < 0 {
		return "", false
	}
	rel := file[idx+len(marker):]
	rel = strings.TrimSuffix(rel, ".java")
	return strings.ReplaceAll(rel, "/", "."), true
}

// targetClasses maps scope.Files to their FQNs via fqnFor, dropping
// anything outside the Maven convention, deduplicated and sorted.
func targetClasses(files []string) []string {
	var classes []string
	for _, f := range files {
		if fqn, ok := fqnFor(f); ok {
			classes = append(classes, fqn)
		}
	}
	return strutil.DedupSorted(classes)
}

// Run invokes PIT's Maven plugin goal directly (no full `mvn test`
// lifecycle first — the goal runs the project's tests itself) scoped
// to scope.Files' FQNs, then parses whichever mutations.xml the run
// produced.
func (Tool) Run(ctx context.Context, repo detect.Repo, scope mutation.Scope, x exec.Execer, timeout time.Duration) (mutation.Report, error) {
	classes := targetClasses(scope.Files)
	if len(classes) == 0 {
		return mutation.Report{}, nil
	}

	full := filepath.Join(repo.LocalPath, reportsDir)
	defer func() { _ = os.RemoveAll(full) }()

	args := []string{
		"org.pitest:pitest-maven:mutationCoverage",
		"-DtargetClasses=" + strings.Join(classes, ","),
		"-DoutputFormats=XML",
	}
	_, err := x.Run(ctx, "mvn", args, exec.RunOpts{Dir: repo.LocalPath, Timeout: timeout})
	if err != nil && !exec.IsToolFailed(err) {
		return mutation.Report{}, err
	}

	xmlPath, ferr := findMutationsXML(full)
	if ferr != nil {
		return mutation.Report{}, ferr
	}
	data, rerr := os.ReadFile(xmlPath) //nolint:gosec // G304: xmlPath is found under repo.LocalPath, an in-scope repository ai-flow already resolved
	if rerr != nil {
		return mutation.Report{}, fmt.Errorf("pitest: read report: %w", rerr)
	}
	return Parse(data)
}

// findMutationsXML locates the one mutations.xml PIT wrote under dir
// (dir/<timestamp>/mutations.xml — PIT names the timestamped
// subdirectory itself, so this walks rather than guessing the name).
func findMutationsXML(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("pitest: read %s: %w", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(dir, e.Name(), "mutations.xml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("pitest: no mutations.xml found under %s", dir)
}

// pitestMutations mirrors PIT's own mutations.xml shape: a flat list
// of <mutation> elements, each carrying its own detected status,
// source file, line and mutator.
type pitestMutations struct {
	Mutations []struct {
		Detected   string `xml:"detected,attr"`
		Status     string `xml:"status,attr"`
		SourceFile string `xml:"sourceFile"`
		LineNumber int    `xml:"lineNumber"`
		Mutator    string `xml:"mutator"`
	} `xml:"mutation"`
}

// Parse decodes PIT's mutations.xml. "KILLED" and "TIMED_OUT" both
// count as caught; "SURVIVED" is a named survivor (id synthesized from
// its position, since PIT's own report carries no stable mutant id);
// "NO_COVERAGE", "MEMORY_ERROR" and "RUN_ERROR" are excluded from
// Total — none of them is evidence about the suite's own adequacy for
// a mutant the run could not properly evaluate.
func Parse(data []byte) (mutation.Report, error) {
	var m pitestMutations
	if err := xml.Unmarshal(data, &m); err != nil {
		return mutation.Report{}, fmt.Errorf("pitest: parse mutations.xml: %w", err)
	}

	out := mutation.Report{}
	for i, mu := range m.Mutations {
		switch mu.Status {
		case "KILLED", "TIMED_OUT":
			out.Total++
			out.Killed++
		case "SURVIVED":
			out.Total++
			out.Survivors = append(out.Survivors, mutation.Survivor{
				ID:   "pitest-" + strconv.Itoa(i),
				File: path.Base(mu.SourceFile),
				Line: mu.LineNumber,
			})
		case "NO_COVERAGE", "MEMORY_ERROR", "RUN_ERROR":
			// Excluded from Total — see this function's own doc comment.
		default:
			return mutation.Report{}, fmt.Errorf("pitest: parse mutations.xml: unrecognized status %q", mu.Status)
		}
	}
	return out, nil
}
