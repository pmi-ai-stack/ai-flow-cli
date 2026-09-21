package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// kubeconfigDoc is the minimal shape Validate reads from a kubeconfig
// file: just the context names. Not a full kubeconfig client — bringing
// in one for a single field would cost more than it is worth.
type kubeconfigDoc struct {
	Contexts []struct {
		Name string `yaml:"name"`
	} `yaml:"contexts"`
}

// kubeconfigContexts reads path and returns its context names. A
// missing or unparsable file is not hidden here: Validate turns the
// error into a warning, per the spec's own "(warning)" on this rule.
func kubeconfigContexts(path string) ([]string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is $KUBECONFIG/~/.kube/config, resolved by DefaultKubeconfigPath, not attacker input
	if err != nil {
		return nil, err
	}
	var doc kubeconfigDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(doc.Contexts))
	for _, c := range doc.Contexts {
		names = append(names, c.Name)
	}
	return names, nil
}

// DefaultKubeconfigPath resolves $KUBECONFIG, falling back to
// ~/.kube/config — the same fallback the spec's Secrets table
// documents. Callers pass its result to Validate; Validate itself never
// reads the environment or the OS home directory, so its tests never
// depend on either being real.
func DefaultKubeconfigPath() string {
	if v := os.Getenv("KUBECONFIG"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kube", "config")
}
