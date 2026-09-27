//go:build ops

package harness_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/harness"
)

// TestRealKindClusterLifecycle is the spec's own "the only place
// ai-flow creates anything, and it deletes it" claim, checked against
// a real `kind` binary and a real Docker daemon for the first time in
// this codebase's history — every prior OPS-tagged phase (21, 22, 24,
// 25, 26, 27) recorded kind as absent from its own development
// environment and left this path unverified. Skips, never fails, when
// either kind or a reachable Docker daemon is absent (the same posture
// every other `-tags ops` real-tool test in this module already uses).
//
// Real cost disclosed up front: creating a kind cluster pulls a ~1GB
// node image on a cold Docker cache and takes roughly a minute even
// warm — this is why the nightly nightly.yml job (not the per-PR one)
// is where this tag runs in CI.
// clusterOpTimeout bounds every kind invocation this test makes — one
// named constant rather than six repeated literals.
const clusterOpTimeout = 3 * time.Minute

func TestRealKindClusterLifecycle(t *testing.T) {
	if _, ok := exec.Which("kind"); !ok {
		t.Skip("kind not on PATH")
	}
	if _, ok := exec.Which("docker"); !ok {
		t.Skip("docker not on PATH")
	}

	x := exec.NewRunner()
	ctx := context.Background()
	home := t.TempDir()
	key := "PHASE30-1"

	t.Cleanup(func() {
		_ = harness.TeardownCluster(context.Background(), x, home, key, clusterOpTimeout)
	})

	exists, err := harness.ClusterExists(ctx, x, key)
	if err != nil {
		t.Fatalf("ClusterExists (before): %v", err)
	}
	if exists {
		t.Fatalf("cluster %q already exists before this test created it — a leaked cluster from a prior run", harness.ClusterName(key))
	}

	if err := harness.EnsureCluster(ctx, x, home, key, clusterOpTimeout); err != nil {
		t.Fatalf("EnsureCluster: %v", err)
	}

	exists, err = harness.ClusterExists(ctx, x, key)
	if err != nil {
		t.Fatalf("ClusterExists (after create): %v", err)
	}
	if !exists {
		t.Fatal("ClusterExists = false after EnsureCluster succeeded")
	}

	kubeconfig := harness.KubeconfigPath(home, key)
	if _, err := os.Stat(kubeconfig); err != nil {
		t.Errorf("kubeconfig %s was not written: %v", kubeconfig, err)
	}

	// EnsureCluster is documented as lazy/idempotent: a second call
	// against an already-existing cluster must be a cheap no-op, not a
	// second `kind create cluster` (which would itself fail against a
	// name already in use).
	if err := harness.EnsureCluster(ctx, x, home, key, clusterOpTimeout); err != nil {
		t.Fatalf("EnsureCluster (second call, already exists): %v", err)
	}

	if err := harness.TeardownCluster(ctx, x, home, key, clusterOpTimeout); err != nil {
		t.Fatalf("TeardownCluster: %v", err)
	}

	exists, err = harness.ClusterExists(ctx, x, key)
	if err != nil {
		t.Fatalf("ClusterExists (after teardown): %v", err)
	}
	if exists {
		t.Error("cluster still exists after TeardownCluster")
	}
	if _, err := os.Stat(kubeconfig); !os.IsNotExist(err) {
		t.Errorf("kubeconfig %s was not removed by TeardownCluster: err=%v", kubeconfig, err)
	}

	// TeardownCluster is documented as a safe no-op when there is
	// nothing to delete — called unconditionally by `stage complete
	// infra-test` on every exit path.
	if err := harness.TeardownCluster(ctx, x, home, key, clusterOpTimeout); err != nil {
		t.Fatalf("TeardownCluster (second call, already absent): %v", err)
	}
}
