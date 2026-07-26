package flatpak

import "testing"

// A reverse-DNS app ID is what routes an install to Flatpak without a
// flag, so it has to be recognised precisely: too loose and ordinary
// package names get sent to Flathub by mistake.
func TestLooksLikeAppID(t *testing.T) {
	yes := []string{
		"org.mozilla.firefox",
		"com.spotify.Client",
		"app.zen_browser.zen",
		"io.github.hrkfdn.ncspot",
		"org.gnome.Calculator",
	}
	for _, s := range yes {
		if !LooksLikeAppID(s) {
			t.Errorf("LooksLikeAppID(%q) = false, want true", s)
		}
	}

	no := []string{
		"firefox",               // ordinary package name
		"pipes.sh",              // AUR package with one dot
		"gtk+3",                 // Void package
		"python3-pip",           //
		"foo.bar",               // only one dot
		"",                      //
		"a..b",                  // empty component
		".leading",              //
		"trailing.",             //
		"has space.a.b",         //
		"some/path.a.b",         // path, not an ID
		"pkg-1.0_1.x86_64.xbps", // package file
		"hello_2.8-4_amd64.deb",
		"foo-1.0-1.fc44.x86_64.rpm",
		"licenses-20240728-1-any.pkg.tar.zst",
	}
	for _, s := range no {
		if LooksLikeAppID(s) {
			t.Errorf("LooksLikeAppID(%q) = true, want false", s)
		}
	}
}

// flatpak prints "No matches found" on stdout and still exits 0, so a
// no-result search used to come back as a package literally named
// "No matches found" -- shown in search output, and installable, since a
// single result is auto-selected.
func TestParseColumnsRejectsNoMatchesMessage(t *testing.T) {
	apps := parseColumns("No matches found\n", func(a *App, col int, val string) {})
	if len(apps) != 0 {
		t.Fatalf("got %d apps, want none: %+v", len(apps), apps)
	}
}

func TestParseColumnsSearchRows(t *testing.T) {
	out := "org.mozilla.firefox\t153.0\tFast, Private & Safe Web Browser\n" +
		"com.brave.Browser\t1.92.144\tBrave Browser\n"
	apps := parseColumns(out, func(a *App, col int, val string) {
		switch col {
		case 1:
			a.Version = val
		case 2:
			a.Desc = val
		}
	})
	if len(apps) != 2 {
		t.Fatalf("got %d apps, want 2", len(apps))
	}
	if apps[0].ID != "org.mozilla.firefox" || apps[0].Version != "153.0" {
		t.Errorf("apps[0] = %+v", apps[0])
	}
	if apps[0].Desc != "Fast, Private & Safe Web Browser" {
		t.Errorf("desc = %q", apps[0].Desc)
	}
	if apps[1].ID != "com.brave.Browser" {
		t.Errorf("apps[1].ID = %q", apps[1].ID)
	}
}

// A missing trailing column is normal -- not every app reports a version.
func TestParseColumnsHandlesShortRows(t *testing.T) {
	apps := parseColumns("org.gnome.Epiphany\n", func(a *App, col int, val string) {
		if col == 1 {
			a.Version = val
		}
	})
	if len(apps) != 1 || apps[0].ID != "org.gnome.Epiphany" || apps[0].Version != "" {
		t.Errorf("got %+v", apps)
	}
}

func TestParseColumnsIgnoresBlankAndJunk(t *testing.T) {
	out := "\n   \nAucune correspondance trouvée\norg.kde.krita\t5.2\tPainting\n"
	apps := parseColumns(out, func(a *App, col int, val string) {})
	if len(apps) != 1 || apps[0].ID != "org.kde.krita" {
		t.Errorf("got %+v, want only org.kde.krita", apps)
	}
}
