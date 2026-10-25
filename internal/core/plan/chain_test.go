package plan_test

import (
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/platform/fake"
)

// TestLinkReaches pins the chain rule: intermediate links are followed only
// when their parent directory is a declared legacy root, at most MaxLegacyHops
// of them, and the chain must end exactly at the source.
func TestLinkReaches(t *testing.T) {
	t.Parallel()
	h := fake.New()
	const source = "/repo/skills/review"
	for _, dir := range []string{"/repo/skills/review", "/shared/real", "/shared/nested", "/other", "/second"} {
		if err := h.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for path, text := range map[string]string{
		"/shared/review":        source,
		"/shared/relative":      "../repo/skills/review",
		"/shared/nested/review": source,
		"/other/review":         source,
		"/second/review":        "/shared/review",
		"/second/third":         "/second/review",
		"/shared/elsewhere":     "/repo/skills/browse",
		"/shared/loop":          "/shared/loop",
	} {
		link(t, h, text, path)
	}
	roots := []string{"/repo", "/shared", "/second"}
	for _, tc := range []struct {
		name, link string
		roots      []string
		want       bool
	}{
		{"direct absolute", source, nil, true},
		{"direct relative", "../repo/skills/review", nil, true},
		{"chain without roots", "/shared/review", nil, false},
		{"chain through a root", "/shared/review", roots, true},
		{"relative hop", "/shared/relative", roots, true},
		{"relative first link", "shared/review", roots, true},
		{"hop below a root, not in it", "/shared/nested/review", roots, false},
		{"hop outside the roots", "/other/review", roots, false},
		{"two hops", "/second/review", roots, true},
		{"three hops exceed the bound", "/second/third", roots, false},
		{"chain ends elsewhere", "/shared/elsewhere", roots, false},
		{"missing hop", "/shared/missing", roots, false},
		{"real directory", "/shared/real", roots, false},
		{"cycle", "/shared/loop", roots, false},
	} {
		got, err := plan.LinkReaches(h, "/link", tc.link, source, tc.roots)
		if err != nil || got != tc.want {
			t.Errorf("%s: LinkReaches = %t, %v; want %t", tc.name, got, err, tc.want)
		}
	}
}
