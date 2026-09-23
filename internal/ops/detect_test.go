package ops_test

import (
	"reflect"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/detect"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/testdata/fixtureaddon"
)

func TestDetectAllAtRepoScope(t *testing.T) {
	tf := fixtureaddon.New("terraform", "infra/prod")
	repos := []ops.DetectInput{{Repo: detect.Repo{Name: "acme.infra", LocalPath: "/tmp/acme.infra"}, Changed: nil}}

	got, err := ops.DetectAll([]ops.Addon{tf}, repos, ops.ScopeRepo)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Targets) != 1 {
		t.Fatalf("Targets = %v, want one", got.Targets)
	}
	tgt := got.Targets[0]
	if tgt.Scope != ops.ScopeRepo {
		t.Errorf("Scope = %q, want %q", tgt.Scope, ops.ScopeRepo)
	}
	if tgt.Changed != nil {
		t.Errorf("Changed = %v, want nil at scope repo", *tgt.Changed)
	}
	if tgt.Repo != "acme.infra" {
		t.Errorf("Repo = %q, want acme.infra", tgt.Repo)
	}
	if len(got.AppFiles) != 0 {
		t.Errorf("AppFiles = %v, want empty (nothing changed yet)", got.AppFiles)
	}
}

func TestDetectAllAtDiffScopeClaimsFilesAndComputesAppFiles(t *testing.T) {
	tf := fixtureaddon.New("terraform", "infra/prod")
	changed := []string{"infra/prod/main.tf", "infra/prod/variables.tf", "cmd/main.go", "README.md"}
	repos := []ops.DetectInput{{Repo: detect.Repo{Name: "acme.infra"}, Changed: changed}}

	got, err := ops.DetectAll([]ops.Addon{tf}, repos, ops.ScopeDiff)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Targets) != 1 {
		t.Fatalf("Targets = %v, want one", got.Targets)
	}
	tgt := got.Targets[0]
	if tgt.Changed == nil || !*tgt.Changed {
		t.Errorf("Changed = %v, want true (target's own files changed)", tgt.Changed)
	}
	wantFiles := []string{"infra/prod/main.tf", "infra/prod/variables.tf"}
	if !reflect.DeepEqual(tgt.Files, wantFiles) {
		t.Errorf("Files = %v, want %v", tgt.Files, wantFiles)
	}

	// cmd/main.go is unclaimed and not documentation -> app file.
	// README.md is documentation -> excluded. The two infra files are
	// claimed by the target -> excluded.
	wantApp := []string{"cmd/main.go"}
	if !reflect.DeepEqual(got.AppFiles, wantApp) {
		t.Errorf("AppFiles = %v, want %v", got.AppFiles, wantApp)
	}
}

func TestDetectAllTargetWithNoChangedFilesIsChangedFalse(t *testing.T) {
	tf := fixtureaddon.New("terraform", "infra/prod")
	changed := []string{"cmd/main.go"}
	repos := []ops.DetectInput{{Repo: detect.Repo{Name: "acme.infra"}, Changed: changed}}

	got, err := ops.DetectAll([]ops.Addon{tf}, repos, ops.ScopeDiff)
	if err != nil {
		t.Fatal(err)
	}
	if got.Targets[0].Changed == nil || *got.Targets[0].Changed {
		t.Errorf("Changed = %v, want false — no changed file falls under infra/prod", got.Targets[0].Changed)
	}
}

func TestDetectAllSortsByRepoThenAddonThenPath(t *testing.T) {
	tf := fixtureaddon.New("terraform", "infra/prod")
	kc := fixtureaddon.New("kubectl", "manifests/")
	repos := []ops.DetectInput{
		{Repo: detect.Repo{Name: "b.repo"}, Changed: nil},
		{Repo: detect.Repo{Name: "a.repo"}, Changed: nil},
	}

	got, err := ops.DetectAll([]ops.Addon{kc, tf}, repos, ops.ScopeRepo)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, tgt := range got.Targets {
		order = append(order, tgt.Repo+":"+tgt.Addon)
	}
	want := []string{"a.repo:kubectl", "a.repo:terraform", "b.repo:kubectl", "b.repo:terraform"}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestDetectAllNoAddonsYieldsEmptyTargetsAndFullAppFiles(t *testing.T) {
	changed := []string{"cmd/main.go"}
	repos := []ops.DetectInput{{Repo: detect.Repo{Name: "acme.infra"}, Changed: changed}}

	got, err := ops.DetectAll(nil, repos, ops.ScopeDiff)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Targets) != 0 {
		t.Fatalf("Targets = %v, want empty — DefaultRegistry ships with zero addons", got.Targets)
	}
	if !reflect.DeepEqual(got.AppFiles, []string{"cmd/main.go"}) {
		t.Errorf("AppFiles = %v, want [cmd/main.go]", got.AppFiles)
	}
}
