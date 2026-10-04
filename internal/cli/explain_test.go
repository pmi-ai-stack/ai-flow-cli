package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
)

type explainEnvelope struct {
	OK     bool `json:"ok"`
	Result struct {
		envelope.Explanation
		Codes []envelope.Explanation `json:"codes"`
	} `json:"result"`
	Error *struct{ Code, Message string } `json:"error"`
}

func runExplain(t *testing.T, wantExit int, args ...string) explainEnvelope {
	t.Helper()
	stdout, _, ec := runRoot(t, append([]string{"explain"}, args...)...)
	if ec != wantExit {
		t.Fatalf("explain %v: exitCode = %d, want %d\n%s", args, ec, wantExit, stdout.String())
	}
	var env explainEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("explain %v: stdout is not valid JSON: %v\n%s", args, err, stdout.String())
	}
	return env
}

func TestExplainOneCode(t *testing.T) {
	env := runExplain(t, 0, "base_diverged")
	r := env.Result
	if r.Code != "base_diverged" || r.ExitCode != envelope.ExitNeedsInput || r.Meaning == "" || r.Recovery == "" {
		t.Fatalf("result = %+v, want base_diverged / exit 4 with prose", r.Explanation)
	}
	if r.Codes != nil {
		t.Errorf("a single-code answer carries a code list: %v", r.Codes)
	}
}

func TestExplainEveryKnownCode(t *testing.T) {
	for _, code := range envelope.KnownCodes() {
		if env := runExplain(t, 0, code); env.Result.Code != code {
			t.Errorf("explain %s answered %q", code, env.Result.Code)
		}
	}
}

func TestExplainWithoutCodeListsAll(t *testing.T) {
	env := runExplain(t, 0)
	if got, want := len(env.Result.Codes), len(envelope.KnownCodes()); got != want {
		t.Fatalf("listed %d codes, want %d", got, want)
	}
	if env.Result.Code != "" {
		t.Errorf("a list answer names a single code %q", env.Result.Code)
	}
	if first := env.Result.Codes[0]; first.ExitCode != envelope.ExitInternal {
		t.Errorf("first entry %s has exit %d; the list starts at exit 1", first.Code, first.ExitCode)
	}
}

func TestExplainUnknownCodeIsUsage(t *testing.T) {
	env := runExplain(t, 2, "no_such_code")
	if env.Error == nil || env.Error.Code != "usage" {
		t.Fatalf("error = %+v, want usage", env.Error)
	}
}

// explain reads no configuration, home or credentials: it must work on
// a machine where nothing is set up.
func TestExplainNeedsNoHome(t *testing.T) {
	runExplain(t, 0, "missing_artifact", "--home", t.TempDir()+"/does/not/exist")
}

// Cobra's own argument-count error is returned from Execute and cmd/ai-flow
// turns it into exit 2, like every other command's.
func TestExplainTooManyArgsIsRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	ec := 0
	root := NewRoot(fixedBuildInfo, strings.NewReader(""), &stdout, &stderr, &ec)
	root.SetArgs([]string{"explain", "usage", "internal"})
	if err := root.ExecuteContext(context.Background()); err == nil {
		t.Fatal("explain with two codes succeeded")
	}
}
