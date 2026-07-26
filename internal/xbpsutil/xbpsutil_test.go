package xbpsutil

import (
	"errors"
	"strings"
	"testing"
)

// searchLineRe parses `xbps-query -Rs` output. Getting the marker wrong
// would mislabel installed packages; getting the pkgver field wrong would
// feed garbage to the batched name resolution.
func TestSearchLineRe(t *testing.T) {
	cases := []struct {
		line           string
		match          bool
		marker, pkgver string
		desc           string
	}{
		{"[*] firefox-153.0_1                     Mozilla Firefox web browser", true, "*", "firefox-153.0_1", "Mozilla Firefox web browser"},
		{"[-] ffsend-0.2.77_2                     Fully featured Firefox Send client", true, "-", "ffsend-0.2.77_2", "Fully featured Firefox Send client"},
		// No description at all.
		{"[-] tiny-1.0_1", true, "-", "tiny-1.0_1", ""},
		// Names legitimately containing dashes and dots.
		{"[*] gtk+3-3.24.52_1  desc", true, "*", "gtk+3-3.24.52_1", "desc"},
		{"[-] python3-pip-24.0_1  desc", true, "-", "python3-pip-24.0_1", "desc"},
		// Not search output.
		{"", false, "", "", ""},
		{"random text", false, "", "", ""},
		{"[?] weird-1.0_1 desc", false, "", "", ""},
	}
	for _, c := range cases {
		m := searchLineRe.FindStringSubmatch(c.line)
		if (m != nil) != c.match {
			t.Errorf("%q: match = %v, want %v", c.line, m != nil, c.match)
			continue
		}
		if m == nil {
			continue
		}
		if m[1] != c.marker || m[2] != c.pkgver {
			t.Errorf("%q -> marker %q pkgver %q, want %q / %q", c.line, m[1], m[2], c.marker, c.pkgver)
		}
		if got := trimSpace(m[3]); got != c.desc {
			t.Errorf("%q -> desc %q, want %q", c.line, got, c.desc)
		}
	}
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

func TestSplitPkgver(t *testing.T) {
	cases := []struct{ in, name, version string }{
		{"firefox-153.0_1", "firefox", "153.0_1"},
		// pkgnames legitimately contain dashes and digits.
		{"otter-browser-1.0.03_1", "otter-browser", "1.0.03_1"},
		{"font-firacode-6.2_2", "font-firacode", "6.2_2"},
		{"libio.elementary.files-devel-6.5.3_1", "libio.elementary.files-devel", "6.5.3_1"},
		{"nodash", "nodash", ""},
	}
	for _, tc := range cases {
		name, version := splitPkgver(tc.in)
		if name != tc.name || version != tc.version {
			t.Errorf("splitPkgver(%q) = (%q, %q), want (%q, %q)", tc.in, name, version, tc.name, tc.version)
		}
	}
}

func TestPkgFileRe(t *testing.T) {
	cases := []struct{ file, name, version, arch string }{
		{"vpa-1.22_1.x86_64.xbps", "vpa", "1.22_1", "x86_64"},
		{"vpa-1.21_1.x86_64-musl.xbps", "vpa", "1.21_1", "x86_64-musl"},
		// pkgnames contain dashes; the revision suffix never does.
		{"otter-browser-1.0.03_1.x86_64.xbps", "otter-browser", "1.0.03_1", "x86_64"},
		{"font-firacode-6.2_2.aarch64.xbps", "font-firacode", "6.2_2", "aarch64"},
	}
	for _, tc := range cases {
		m := pkgFileRe.FindStringSubmatch(tc.file)
		if m == nil {
			t.Errorf("%s did not match", tc.file)
			continue
		}
		if m[1] != tc.name || m[2] != tc.version || m[3] != tc.arch {
			t.Errorf("%s => (%q, %q, %q), want (%q, %q, %q)",
				tc.file, m[1], m[2], m[3], tc.name, tc.version, tc.arch)
		}
	}
}

// The listing is HTML from a web server, so the regex has to pull package
// names out of markup without dragging in the surrounding tags.
func TestRemoteVersionsParsesDirectoryListing(t *testing.T) {
	arch, err := Arch()
	if err != nil {
		t.Skip("no xbps arch available")
	}
	listing := `<html><body>
<a href="vpa-1.20_1.` + arch + `.xbps">vpa-1.20_1.` + arch + `.xbps</a>
<a href="vpa-1.22_1.` + arch + `.xbps">vpa-1.22_1.` + arch + `.xbps</a>
<a href="vpa-1.21_1.` + arch + `.xbps">vpa-1.21_1.` + arch + `.xbps</a>
<a href="vpa-1.22_1.someotherarch.xbps">wrong arch</a>
<a href="vpatools-9.9_1.` + arch + `.xbps">different package</a>
</body></html>`

	got := RemoteVersions("vpa", func(string) (string, error) { return listing, nil })
	if len(got) != 3 {
		t.Fatalf("got %d versions, want 3: %+v", len(got), got)
	}
	// Newest first, so the picker offers the least-drastic rollback first.
	want := []string{"1.22_1", "1.21_1", "1.20_1"}
	for i, w := range want {
		if got[i].Version != w {
			t.Errorf("versions[%d] = %q, want %q", i, got[i].Version, w)
		}
	}
	if !strings.HasSuffix(got[0].Path, "vpa-1.22_1."+arch+".xbps") {
		t.Errorf("Path = %q, missing the filename", got[0].Path)
	}
}

func TestRemoteVersionsSurvivesUnreachableRepo(t *testing.T) {
	got := RemoteVersions("vpa", func(string) (string, error) {
		return "", errors.New("connection refused")
	})
	if len(got) != 0 {
		t.Errorf("got %+v, want none", got)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.22_1", "1.21_1", 1},
		{"1.21_1", "1.22_1", -1},
		{"1.22_1", "1.22_1", 0},
		{"1.22_2", "1.22_1", 1},
		// 1.9 vs 1.10 is where a plain string compare gets it wrong.
		{"1.10_1", "1.9_1", 1},
	}
	for _, tc := range cases {
		if got := CompareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
