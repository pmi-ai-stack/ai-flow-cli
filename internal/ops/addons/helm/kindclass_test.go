package helm

import (
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/ops"
)

func TestClassifyKind(t *testing.T) {
	cases := []struct {
		kind string
		want ops.KindClass
	}{
		{"Secret", ops.KindSecret},
		{"Deployment", ops.KindWorkload},
		{"Service", ops.KindNetwork},
		{"ConfigMap", ops.KindOther},
		{"MyCustomResource", ops.KindOther},
	}
	for _, c := range cases {
		if got := classifyKind(c.kind); got != c.want {
			t.Errorf("classifyKind(%q) = %q, want %q", c.kind, got, c.want)
		}
	}
}
