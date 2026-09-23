package ops_test

import (
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops/testdata/fixtureaddon"
)

func TestDefaultRegistryIsEmpty(t *testing.T) {
	if got := ops.DefaultRegistry().Addons(); len(got) != 0 {
		t.Fatalf("DefaultRegistry().Addons() = %v, want empty — no addon implementation exists before phase 21", got)
	}
}

func TestRegistryAddonsOrderAndLookup(t *testing.T) {
	tf := fixtureaddon.New("terraform")
	kc := fixtureaddon.New("kubectl")
	r := ops.NewRegistry(tf, kc)

	got := r.Addons()
	if len(got) != 2 || got[0].Name() != "terraform" || got[1].Name() != "kubectl" {
		t.Fatalf("Addons() = %v, want [terraform, kubectl] in registration order", names(got))
	}

	if a, ok := r.Lookup("kubectl"); !ok || a.Name() != "kubectl" {
		t.Fatalf("Lookup(kubectl) = %v, %v", a, ok)
	}
	if _, ok := r.Lookup("helm"); ok {
		t.Fatal("Lookup(helm) found an addon that was never registered")
	}
}

func TestRegistryAddonsReturnsACopy(t *testing.T) {
	r := ops.NewRegistry(fixtureaddon.New("terraform"))
	got := r.Addons()
	got[0] = nil
	if a, ok := r.Lookup("terraform"); !ok || a == nil {
		t.Fatal("mutating Addons()'s returned slice affected the registry's own state")
	}
}

func names(addons []ops.Addon) []string {
	out := make([]string, len(addons))
	for i, a := range addons {
		out[i] = a.Name()
	}
	return out
}
