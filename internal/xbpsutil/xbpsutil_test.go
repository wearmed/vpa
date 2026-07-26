package xbpsutil

import "testing"

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
