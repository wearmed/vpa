package deps

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseIntoSkipsCommentsAndBlanks(t *testing.T) {
	m := map[string]string{}
	parseInto(m, []byte(`# a comment
gtk2=gtk+

  # indented comment
qt5-base=qt5
systemd=-
notakeyvalueline
`))
	if m["gtk2"] != "gtk+" || m["qt5-base"] != "qt5" {
		t.Errorf("mappings not parsed: %v", m)
	}
	if m["systemd"] != "-" {
		t.Errorf(`"-" (no Void equivalent) must be preserved, got %q`, m["systemd"])
	}
	if _, ok := m["notakeyvalueline"]; ok {
		t.Error("a line with no '=' should be ignored")
	}
	if _, ok := m["# a comment"]; ok {
		t.Error("comments must be ignored")
	}
}

func TestParseIntoLaterWins(t *testing.T) {
	m := map[string]string{}
	parseInto(m, []byte("foo=one\nfoo=two\n"))
	if m["foo"] != "two" {
		t.Errorf("later entry should win, got %q", m["foo"])
	}
}

// The embedded default map must be well-formed and contain the entries the
// rest of vpa relies on.
func TestEmbeddedDepmapIsSane(t *testing.T) {
	m := map[string]string{}
	parseInto(m, defaultDepmap)
	if len(m) == 0 {
		t.Fatal("embedded depmap.conf parsed to nothing")
	}
	if m["gtk3"] != "gtk+3" {
		t.Errorf("gtk3 -> %q, want gtk+3", m["gtk3"])
	}
	// systemd has no Void equivalent and must stay explicitly unresolvable
	// rather than being mapped to something merely similar.
	for _, k := range []string{"systemd", "systemd-libs", "libsystemd"} {
		if m[k] != "-" {
			t.Errorf("%s -> %q, want %q", k, m[k], "-")
		}
	}
	for k, v := range m {
		if k == "" || v == "" {
			t.Errorf("empty key or value: %q=%q", k, v)
		}
	}
}

func TestDepmapLookupUserOverrides(t *testing.T) {
	dir := t.TempDir()
	userMap := filepath.Join(dir, "depmap.conf")
	if err := os.WriteFile(userMap, []byte("gtk3=my-custom-gtk\nbrandnew=mapped\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Reset the package-level cache so this test controls what gets loaded.
	resetDepmapForTest()

	if got := DepmapLookup(userMap, "gtk3"); got != "my-custom-gtk" {
		t.Errorf("user override ignored: gtk3 -> %q", got)
	}
	if got := DepmapLookup(userMap, "brandnew"); got != "mapped" {
		t.Errorf("user-only entry ignored: %q", got)
	}
	// Unmapped names pass through unchanged.
	if got := DepmapLookup(userMap, "somethingelse"); got != "somethingelse" {
		t.Errorf("unmapped name should pass through, got %q", got)
	}
	// Defaults still apply where the user didn't override.
	if got := DepmapLookup(userMap, "systemd"); got != "-" {
		t.Errorf("default entry lost: systemd -> %q", got)
	}
	resetDepmapForTest()
}

func TestTiersOrdersDependenciesFirst(t *testing.T) {
	r := NewResolver("", "")
	// c depends on b, b depends on a; PlanOrder is post-order.
	r.PlanOrder = []string{"a", "b", "c"}
	r.Edges["b"] = []string{"a"}
	r.Edges["c"] = []string{"b"}

	tiers := r.Tiers()
	if len(tiers) != 3 {
		t.Fatalf("expected 3 tiers for a chain, got %d: %v", len(tiers), tiers)
	}
	if tiers[0][0] != "a" || tiers[1][0] != "b" || tiers[2][0] != "c" {
		t.Errorf("tiers out of dependency order: %v", tiers)
	}
}

func TestTiersGroupsIndependentPackages(t *testing.T) {
	r := NewResolver("", "")
	r.PlanOrder = []string{"a", "b", "c"}
	r.Edges["c"] = []string{"a", "b"} // a and b are independent of each other

	tiers := r.Tiers()
	if len(tiers) != 2 {
		t.Fatalf("expected 2 tiers, got %d: %v", len(tiers), tiers)
	}
	if len(tiers[0]) != 2 {
		t.Errorf("independent packages should share a tier: %v", tiers)
	}
	if len(tiers[1]) != 1 || tiers[1][0] != "c" {
		t.Errorf("dependent package should be in a later tier: %v", tiers)
	}
}

func TestTiersEmpty(t *testing.T) {
	if got := NewResolver("", "").Tiers(); len(got) != 0 {
		t.Errorf("empty plan should give no tiers, got %v", got)
	}
}
