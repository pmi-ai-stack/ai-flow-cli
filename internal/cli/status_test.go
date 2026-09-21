package cli

import (
	"encoding/json"
	"testing"
)

type statusEnvelope struct {
	OK     bool `json:"ok"`
	Result struct {
		Key         string  `json:"key"`
		NextStage   *string `json:"next_stage"`
		NextSkill   *string `json:"next_skill"`
		NextCommand *string `json:"next_command"`
		NextState   *string `json:"next_state"`
		HandoffPath string  `json:"handoff_path"`
	} `json:"result"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func decodeStatus(t *testing.T, raw []byte) statusEnvelope {
	t.Helper()
	var env statusEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, raw)
	}
	return env
}

func TestStatusOnEmptyHandoff(t *testing.T) {
	home := t.TempDir()
	stdout, _, exitCode := runRoot(t, "status", "PROJ-1", "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	env := decodeStatus(t, stdout.Bytes())
	if !env.OK {
		t.Fatal("ok = false, want true")
	}
	if env.Result.Key != "PROJ-1" {
		t.Errorf("key = %q, want PROJ-1", env.Result.Key)
	}
	if env.Result.NextStage == nil || *env.Result.NextStage != "fetch" {
		t.Errorf("next_stage = %v, want fetch", env.Result.NextStage)
	}
	if env.Result.NextSkill != nil {
		t.Errorf("next_skill = %v, want null (fetch is CLI-only)", *env.Result.NextSkill)
	}
	if env.Result.NextCommand == nil || *env.Result.NextCommand != "ticket fetch" {
		t.Errorf("next_command = %v, want \"ticket fetch\"", env.Result.NextCommand)
	}
	if env.Result.NextState == nil || *env.Result.NextState != "pending" {
		t.Errorf("next_state = %v, want pending", env.Result.NextState)
	}
}

func TestStatusInvalidKey(t *testing.T) {
	home := t.TempDir()
	_, _, exitCode := runRoot(t, "status", "not-a-key", "--home", home)
	if exitCode != 2 {
		t.Errorf("exitCode = %d, want 2 (usage)", exitCode)
	}
}

func TestStatusAfterFetchCompletes(t *testing.T) {
	home := t.TempDir()
	key := "PROJ-1"
	if _, _, ec := runRoot(t, "stage", "start", key, "fetch", "--home", home); ec != 0 {
		t.Fatalf("stage start: exitCode = %d", ec)
	}
	writeFixture(t, home, key, "ticket", "ticket.json")
	if _, _, ec := runRoot(t, "stage", "complete", key, "fetch", "--home", home); ec != 0 {
		t.Fatalf("stage complete: exitCode = %d", ec)
	}

	stdout, _, exitCode := runRoot(t, "status", key, "--home", home)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0\n%s", exitCode, stdout.String())
	}
	env := decodeStatus(t, stdout.Bytes())
	if env.Result.NextStage == nil || *env.Result.NextStage != "locate" {
		t.Errorf("next_stage = %v, want locate", env.Result.NextStage)
	}
	if env.Result.NextCommand == nil || *env.Result.NextCommand != "repo locate" {
		t.Errorf("next_command = %v, want \"repo locate\"", env.Result.NextCommand)
	}
}
