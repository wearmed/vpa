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
