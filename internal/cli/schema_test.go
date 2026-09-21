package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSchemaCommandJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := -1
	root := NewRoot(fixedBuildInfo, &stdout, &stderr, &exitCode)
	root.SetArgs([]string{"schema", "status"})

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", exitCode)
	}

	var env struct {
		OK     bool `json:"ok"`
		Result struct {
			Title string `json:"title"`
		} `json:"result"`
		Error *struct{} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if !env.OK {
		t.Fatalf("ok = false, want true (error: %v)", env.Error)
	}
	if env.Result.Title != "status" {
		t.Errorf("result.title = %q, want %q", env.Result.Title, "status")
	}
}

func TestSchemaCommandUnknownArtifact(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := -1
	root := NewRoot(fixedBuildInfo, &stdout, &stderr, &exitCode)
	root.SetArgs([]string{"schema", "no-such-artifact"})

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if exitCode != 2 {
		t.Errorf("exitCode = %d, want 2 (usage)", exitCode)
	}

	var env struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code    string   `json:"code"`
			Message string   `json:"message"`
			Details []string `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if env.OK {
		t.Fatal("ok = true, want false")
	}
	if env.Error == nil || env.Error.Code != "usage" {
		t.Fatalf("error = %+v, want code=usage", env.Error)
	}
	if len(env.Error.Details) != 1 || !strings.Contains(env.Error.Details[0], "status") {
		t.Errorf("error.details = %v, want a list naming valid artifacts", env.Error.Details)
	}
}

func TestSchemaCommandRequiresArg(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exitCode := -1
	root := NewRoot(fixedBuildInfo, &stdout, &stderr, &exitCode)
	root.SetArgs([]string{"schema"})

	err := root.ExecuteContext(context.Background())
	if err == nil {
		t.Fatal("Execute: want an error for missing positional arg, got nil")
	}
}
