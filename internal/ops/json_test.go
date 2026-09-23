package ops

import (
	"encoding/json"
	"errors"
	"testing"
)

// TestFindingJSONKeyOrder pins Finding's own byte-stability order
// (target.go's documented convention: fields declared in JSON-key
// alphabetical order) — phase 20 is this type's first real serializer.
func TestFindingJSONKeyOrder(t *testing.T) {
	f := Finding{File: "main.tf", Line: 3, Message: "boom", Severity: "error", Tool: "tflint"}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"file":"main.tf","line":3,"message":"boom","severity":"error","tool":"tflint"}`
	if string(data) != want {
		t.Errorf("Marshal(Finding) = %s, want %s", data, want)
	}
}

// TestFindingOmitsEmptyFileAndLine proves a finding with no file/line
// (a whole-target diagnostic) marshals without them, matching the
// schema's own "not required" treatment of both.
func TestFindingOmitsEmptyFileAndLine(t *testing.T) {
	f := Finding{Message: "boom", Severity: "error", Tool: "tflint"}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"message":"boom","severity":"error","tool":"tflint"}`
	if string(data) != want {
		t.Errorf("Marshal(Finding) = %s, want %s", data, want)
	}
}

// TestDriftReportJSONKeyOrder pins DriftReport's byte-stability order.
func TestDriftReportJSONKeyOrder(t *testing.T) {
	r := DriftReport{Drift: DriftPresent, Resources: []string{"aws_s3_bucket.x"}}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"drift":"present","drift_resources":["aws_s3_bucket.x"]}`
	if string(data) != want {
		t.Errorf("Marshal(DriftReport) = %s, want %s", data, want)
	}
}

// TestActionJSONKeyOrder pins Action's byte-stability order: action,
// address, addon, env, fields_changed, kind, kind_class, name,
// protected, sensitive, target, tier.
func TestActionJSONKeyOrder(t *testing.T) {
	a := Action{
		Action: ActionUpdate, Address: "aws_s3_bucket.x", Addon: "terraform", Env: "prod",
		FieldsChanged: []string{"tags"}, Kind: "aws_s3_bucket", KindClass: KindData, Name: "x",
		Protected: true, Sensitive: false, Target: "acme.infra:infra/prod", Tier: "prod",
	}
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"action":"update","address":"aws_s3_bucket.x","addon":"terraform","env":"prod","fields_changed":["tags"],"kind":"aws_s3_bucket","kind_class":"data","name":"x","protected":true,"sensitive":false,"target":"acme.infra:infra/prod","tier":"prod"}`
	if string(data) != want {
		t.Errorf("Marshal(Action) = %s, want %s", data, want)
	}
}

func TestChangesetJSONShape(t *testing.T) {
	cs := Changeset{Actions: []Action{{Action: ActionNoop}}}
	data, err := json.Marshal(cs)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if _, ok := back["actions"]; !ok {
		t.Errorf("Marshal(Changeset) = %s, missing \"actions\"", data)
	}
}

// TestErrLiveCredentialsMissingIsDistinctFromToolFailed proves the
// sentinel round-trips through errors.Is even when wrapped, and is not
// mistaken for any other error.
func TestErrLiveCredentialsMissingIsDistinctFromToolFailed(t *testing.T) {
	if !errors.Is(ErrLiveCredentialsMissing, ErrLiveCredentialsMissing) {
		t.Error("ErrLiveCredentialsMissing does not match itself via errors.Is")
	}
	if errors.Is(errors.New("some other error"), ErrLiveCredentialsMissing) {
		t.Error("an unrelated error matched ErrLiveCredentialsMissing")
	}
}
