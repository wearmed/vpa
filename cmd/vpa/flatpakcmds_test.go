package main

import (
	"testing"

	"vpa/internal/flatpak"
)

func apps(ids ...string) []flatpak.App {
	var out []flatpak.App
	for _, id := range ids {
		out = append(out, flatpak.App{ID: id})
	}
	return out
}

// Regression: auto-selecting on a last-segment match without requiring it
// to be unique silently installed an arbitrary app -- "calculator" matched
// five Flathub apps and picked whichever came first.
func TestNarrowFlatpakMatchesRefusesAmbiguity(t *testing.T) {
	ambiguous := apps(
		"app.curocalc.calculator",
		"org.gnome.Calculator",
		"io.github.x.calculator",
	)
	id, shortlist := narrowFlatpakMatches("calculator", ambiguous)
	if id != "" {
		t.Errorf("auto-selected %q from an ambiguous name; must ask instead", id)
	}
	if len(shortlist) != 3 {
		t.Errorf("shortlist = %v, want all three exact matches", shortlist)
	}
}

func TestNarrowFlatpakMatchesUniqueExact(t *testing.T) {
	// Only one app's last segment is "Calculator", so it's unambiguous even
	// though other results came back from the search.
	got, _ := narrowFlatpakMatches("calculator", apps(
		"org.gnome.Calculator",
		"io.github.johannesboehler2.BmiCalculator",
		"uno.platform.uno-calculator",
	))
	if got != "org.gnome.Calculator" {
		t.Errorf("got %q, want org.gnome.Calculator", got)
	}
}

func TestNarrowFlatpakMatchesSingleResult(t *testing.T) {
	got, _ := narrowFlatpakMatches("obs", apps("com.obsproject.Studio"))
	if got != "com.obsproject.Studio" {
		t.Errorf("a single result should be taken, got %q", got)
	}
}

func TestNarrowFlatpakMatchesNoExactManyResults(t *testing.T) {
	in := apps("org.kde.kalk", "app.curocalc.calculator2", "x.y.z")
	id, shortlist := narrowFlatpakMatches("calc", in)
	if id != "" {
		t.Errorf("no exact match should not auto-select, got %q", id)
	}
	if len(shortlist) != len(in) {
		t.Errorf("shortlist = %v, want all results", shortlist)
	}
}

func TestNarrowFlatpakMatchesCaseInsensitive(t *testing.T) {
	got, _ := narrowFlatpakMatches("CALCULATOR", apps("org.gnome.Calculator", "a.b.other"))
	if got != "org.gnome.Calculator" {
		t.Errorf("matching should ignore case, got %q", got)
	}
}
