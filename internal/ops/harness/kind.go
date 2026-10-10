// kind.go owns the ticket's own kind cluster — the spec's own "the
// only place ai-flow creates anything, and it deletes it" (the phase's
// own goal statement). Every call here that mutates goes through
// exec.Execer, matched against internal/ops/exec.Table's own three
// "kind" rows (create/delete cluster, both class: ephemeral; get
// clusters, class: none — see the phase-24 plan's design decision 5).
package harness

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/exec"
)

// ClusterName is the ticket's own kind cluster name — the lowercased
// ticket key. handoff.ValidateKey's own pattern
// (^[A-Z][A-Z0-9]*-[0-9]+$) lowercases to a valid kind/DNS-1123 label
// with no further sanitizing needed.
func ClusterName(key string) string {
	return strings.ToLower(key)
}

// KubeContext is the kubeconfig context name `kind create cluster`
// writes for ClusterName(key) — kind's own fixed "kind-<name>"
// convention.
func KubeContext(key string) string {
	return "kind-" + ClusterName(key)
}

// KubeconfigPath is the per-ticket kubeconfig file `kind create
// cluster --kubeconfig` writes to, instead of the developer's own
// default ~/.kube/config (plan design decision 5: kind's own default
// behaviour rewrites the ambient kubeconfig and its current-context, a
// hidden mutation of shared machine state this package has no mandate
// to cause).
func KubeconfigPath(home, key string) string {
	return filepath.Join(home, "cache", "kind", ClusterName(key)+".kubeconfig")
}

// ListClusters lists every kind cluster's own name (kind tracks
// clusters by Docker container label, not by kubeconfig file, so this
// needs no --kubeconfig flag and sees every cluster on the machine,
// this ticket's own or not). A tool_failed error — including a missing
// kind binary — is tolerated as "nothing to list," not propagated:
// ClusterExists/TeardownCluster's own idempotent-no-op contract
// depends on this holding even when kind was never installed at all
// (plan design decision 15).
func ListClusters(ctx context.Context, x exec.Execer) ([]string, error) {
	out, err := x.Run(ctx, "kind", []string{"get", "clusters", "-q"}, exec.RunOpts{})
	if err != nil {
		if exec.IsToolFailed(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for line := range strings.SplitSeq(out.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}

// ClusterExists reports whether ClusterName(key) is among the
// machine's own kind clusters right now.
func ClusterExists(ctx context.Context, x exec.Execer, key string) (bool, error) {
	names, err := ListClusters(ctx, x)
	if err != nil {
		return false, err
	}
	return slices.Contains(names, ClusterName(key)), nil
}

// EnsureCluster creates the ticket's own kind cluster when it does not
// already exist ("lazily," the spec's own word) — its own kubeconfig
// file only, never the developer's default one. Unlike
// ListClusters/TeardownCluster, a real failure here (including a
// missing kind binary) is propagated, not tolerated: a caller that
// reached this point already decided an e2e harness needs the cluster,
// so silently reporting success would hide a real problem.
func EnsureCluster(ctx context.Context, x exec.Execer, home, key string, timeout time.Duration) error {
	exists, err := ClusterExists(ctx, x, key)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	kubeconfig := KubeconfigPath(home, key)
	if err := os.MkdirAll(filepath.Dir(kubeconfig), 0o700); err != nil {
		return err
	}
	_, err = x.Run(ctx, "kind", []string{"create", "cluster", "--name", ClusterName(key), "--kubeconfig", kubeconfig}, exec.RunOpts{Timeout: timeout})
	return err
}

// TeardownCluster deletes the ticket's own kind cluster when it
// exists, and its own kubeconfig file with it; a no-op — never an
// error — when there is nothing to delete, including when kind is not
// installed at all (plan design decision 15: called unconditionally by
// `stage complete KEY infra-test` on both pass and fail, and by
// `stage reset KEY infra-test`, so it must be safe on every ticket,
// not only ones that ever ran an e2e harness).
func TeardownCluster(ctx context.Context, x exec.Execer, home, key string, timeout time.Duration) error {
	exists, err := ClusterExists(ctx, x, key)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	kubeconfig := KubeconfigPath(home, key)
	if _, err := x.Run(ctx, "kind", []string{"delete", "cluster", "--name", ClusterName(key), "--kubeconfig", kubeconfig}, exec.RunOpts{Timeout: timeout}); err != nil {
		return err
	}
	_ = os.Remove(kubeconfig) // best-effort: a leftover empty kubeconfig file is harmless
	return nil
}
