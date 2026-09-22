package visual_test

import (
	"slices"
	"testing"

	"github.com/pmi-ai-stack/ai-flow-cli/internal/verify/visual"
)

func TestSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/", "home"},
		{"", "home"},
		{"/dashboard", "dashboard"},
		{"/dashboard/settings", "dashboard_settings"},
		{"dashboard/settings/", "dashboard_settings"},
	}
	for _, c := range cases {
		if got := visual.Slug(c.in); got != c.want {
			t.Errorf("Slug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDefaultScreens(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  []string
	}{
		{"pages route", []string{"src/pages/dashboard.tsx"}, []string{"/dashboard"}},
		{"pages index", []string{"src/pages/index.tsx"}, []string{"/"}},
		{"nested routes", []string{"app/routes/settings/profile.tsx"}, []string{"/settings/profile"}},
		{"no match falls back to home", []string{"src/lib/utils.ts"}, []string{"/"}},
		{"empty falls back to home", nil, []string{"/"}},
		{
			"multiple distinct screens, deduped",
			[]string{"src/pages/dashboard.tsx", "src/pages/dashboard.tsx", "src/pages/settings.tsx"},
			[]string{"/dashboard", "/settings"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := visual.DefaultScreens(c.files)
			if !slices.Equal(got, c.want) {
				t.Errorf("DefaultScreens(%v) = %v, want %v", c.files, got, c.want)
			}
		})
	}
}
