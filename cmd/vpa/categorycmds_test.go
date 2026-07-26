package main

import "testing"

func TestIsCategoryWord(t *testing.T) {
	for _, yes := range []string{"cat", "category", "categories", "CAT", "Category"} {
		if !isCategoryWord(yes) {
			t.Errorf("isCategoryWord(%q) = false", yes)
		}
	}
	// "cats" is a plausible thing to actually search for.
	for _, no := range []string{"cats", "catalog", "firefox", ""} {
		if isCategoryWord(no) {
			t.Errorf("isCategoryWord(%q) = true", no)
		}
	}
}

func TestSplitCandidates(t *testing.T) {
	pkgs, ids := splitCandidates([]string{
		"firefox", "org.mozilla.firefox", "brave-bin", "io.gitlab.librewolf-community", "w3m",
	})
	wantPkgs := []string{"firefox", "brave-bin", "w3m"}
	wantIDs := []string{"org.mozilla.firefox", "io.gitlab.librewolf-community"}

	if len(pkgs) != len(wantPkgs) {
		t.Fatalf("packages = %v, want %v", pkgs, wantPkgs)
	}
	for i, w := range wantPkgs {
		if pkgs[i] != w {
			t.Errorf("packages[%d] = %q, want %q", i, pkgs[i], w)
		}
	}
	if len(ids) != len(wantIDs) {
		t.Fatalf("ids = %v, want %v", ids, wantIDs)
	}
	for i, w := range wantIDs {
		if ids[i] != w {
			t.Errorf("ids[%d] = %q, want %q", i, ids[i], w)
		}
	}
}

// A package name with dots in it (they exist: pipes.sh) must not be mistaken
// for a Flatpak application ID and looked up in the wrong place.
func TestSplitCandidatesDottedPackageName(t *testing.T) {
	pkgs, ids := splitCandidates([]string{"pipes.sh", "llama.cpp"})
	if len(ids) != 0 {
		t.Errorf("ids = %v, want none", ids)
	}
	if len(pkgs) != 2 {
		t.Errorf("packages = %v, want both", pkgs)
	}
}
