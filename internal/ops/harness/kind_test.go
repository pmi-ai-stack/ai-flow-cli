package harness_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/envelope"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec/exectest"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/harness"
)

func TestClusterNameAndKubeContext(t *testing.T) {
	if got := harness.ClusterName("PROJ-123"); got != "proj-123" {
		t.Errorf("ClusterName = %q, want proj-123", got)
	}
	if got := harness.KubeContext("PROJ-123"); got != "kind-proj-123" {
		t.Errorf("KubeContext = %q, want kind-proj-123", got)
	}
}

func TestKubeconfigPathIsPerTicketNotAmbientDefault(t *testing.T) {
	got := harness.KubeconfigPath("/home/dev/.ai-flow", "PROJ-123")
	want := filepath.Join("/home/dev/.ai-flow", "cache", "kind", "proj-123.kubeconfig")
	if got != want {
		t.Errorf("KubeconfigPath = %q, want %q", got, want)
	}
}

func TestEnsureClusterSkipsCreationWhenAlreadyPresent(t *testing.T) {
	s := exectest.New(t).On("kind", []string{"get", "clusters", "-q"}, exec.Output{Stdout: "proj-123\nother-9\n"})
	home := t.TempDir()
	if err := harness.EnsureCluster(context.Background(), s, home, "PROJ-123", time.Minute); err != nil {
		t.Fatalf("EnsureCluster: %v", err)
	}
	if len(s.Calls) != 1 {
		t.Errorf("Calls = %+v, want only the existence check, no create", s.Calls)
	}
}

func TestEnsureClusterCreatesWithItsOwnKubeconfig(t *testing.T) {
	home := t.TempDir()
	kubeconfig := harness.KubeconfigPath(home, "PROJ-123")
	s := exectest.New(t).
		On("kind", []string{"get", "clusters", "-q"}, exec.Output{Stdout: ""}).
		On("kind", []string{"create", "cluster", "--name", "proj-123", "--kubeconfig", kubeconfig}, exec.Output{})

	if err := harness.EnsureCluster(context.Background(), s, home, "PROJ-123", time.Minute); err != nil {
		t.Fatalf("EnsureCluster: %v", err)
	}
	if len(s.Calls) != 2 {
		t.Fatalf("Calls = %+v, want existence check then create", s.Calls)
	}
	if s.Calls[1].Tool != "kind" || s.Calls[1].Args[0] != "create" {
		t.Errorf("second call = %+v, want a kind create call", s.Calls[1])
	}
	if _, err := os.Stat(filepath.Dir(kubeconfig)); err != nil {
		t.Errorf("kubeconfig parent directory was not created: %v", err)
	}
}

func TestEnsureClusterCreationFailurePropagates(t *testing.T) {
	home := t.TempDir()
	kubeconfig := harness.KubeconfigPath(home, "PROJ-123")
	s := exectest.New(t).
		On("kind", []string{"get", "clusters", "-q"}, exec.Output{Stdout: ""}).
		OnFail("kind", []string{"create", "cluster", "--name", "proj-123", "--kubeconfig", kubeconfig},
			&envelope.Error{Code: "tool_failed", Message: "kind: command not found"})

	err := harness.EnsureCluster(context.Background(), s, home, "PROJ-123", time.Minute)
	if err == nil {
		t.Fatal("EnsureCluster: want the creation failure to propagate, got nil")
	}
}

func TestTeardownClusterNoOpsWhenAbsent(t *testing.T) {
	s := exectest.New(t).On("kind", []string{"get", "clusters", "-q"}, exec.Output{Stdout: "other-9\n"})
	home := t.TempDir()
	if err := harness.TeardownCluster(context.Background(), s, home, "PROJ-123", time.Minute); err != nil {
		t.Fatalf("TeardownCluster: %v", err)
	}
	if len(s.Calls) != 1 {
		t.Errorf("Calls = %+v, want only the existence check, no delete", s.Calls)
	}
}

// TestTeardownClusterNoOpsWhenKindMissing proves the idempotent-safe
// contract (plan design decision 15): a ticket that never touched
// infra-test at all must not fail `stage complete`/`stage reset`
// because kind was never installed.
func TestTeardownClusterNoOpsWhenKindMissing(t *testing.T) {
	s := exectest.New(t).OnFail("kind", []string{"get", "clusters", "-q"},
		&envelope.Error{Code: "tool_failed", Message: "exec: \"kind\": executable file not found in $PATH"})
	home := t.TempDir()
	if err := harness.TeardownCluster(context.Background(), s, home, "PROJ-123", time.Minute); err != nil {
		t.Fatalf("TeardownCluster: %v, want a missing kind binary tolerated as nothing-to-tear-down", err)
	}
}

func TestTeardownClusterDeletesAndRemovesKubeconfig(t *testing.T) {
	home := t.TempDir()
	kubeconfig := harness.KubeconfigPath(home, "PROJ-123")
	if err := os.MkdirAll(filepath.Dir(kubeconfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kubeconfig, []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}

	s := exectest.New(t).
		On("kind", []string{"get", "clusters", "-q"}, exec.Output{Stdout: "proj-123\n"}).
		On("kind", []string{"delete", "cluster", "--name", "proj-123", "--kubeconfig", kubeconfig}, exec.Output{})

	if err := harness.TeardownCluster(context.Background(), s, home, "PROJ-123", time.Minute); err != nil {
		t.Fatalf("TeardownCluster: %v", err)
	}
	if _, err := os.Stat(kubeconfig); !os.IsNotExist(err) {
		t.Errorf("kubeconfig file still exists after teardown: err=%v", err)
	}
}

func TestListClustersToleratesToolFailedAsEmpty(t *testing.T) {
	s := exectest.New(t).OnFail("kind", []string{"get", "clusters", "-q"}, &envelope.Error{Code: "tool_failed", Message: "no clusters"})
	names, err := harness.ListClusters(context.Background(), s)
	if err != nil {
		t.Fatalf("ListClusters: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("ListClusters = %v, want empty", names)
	}
}

func TestListClustersExecNotAllowedPropagates(t *testing.T) {
	s := exectest.New(t).OnFail("kind", []string{"get", "clusters", "-q"}, exec.NotAllowed([]string{"kind", "get", "clusters", "-q"}))
	_, err := harness.ListClusters(context.Background(), s)
	if err == nil {
		t.Fatal("ListClusters: want exec_not_allowed to propagate")
	}
}
