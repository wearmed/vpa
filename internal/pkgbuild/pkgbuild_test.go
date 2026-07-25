package pkgbuild

import "testing"

func TestStripVersionAndDepVersion(t *testing.T) {
	cases := []struct{ dep, name, ver string }{
		{"bash", "bash", ""},
		{"bash>=4.0.0", "bash", ">=4.0.0"},
		{"glibc<=2.41", "glibc", "<=2.41"},
		{"foo=1.0", "foo", "=1.0"},
		{"libx11>1.2", "libx11", ">1.2"},
		{"python3-pip", "python3-pip", ""},
		// A name containing digits and dashes must not be split early.
		{"gtk+3>=3.24", "gtk+3", ">=3.24"},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := StripVersion(c.dep); got != c.name {
			t.Errorf("StripVersion(%q) = %q, want %q", c.dep, got, c.name)
		}
		if got := DepVersion(c.dep); got != c.ver {
			t.Errorf("DepVersion(%q) = %q, want %q", c.dep, got, c.ver)
		}
	}
}

func TestOptDepName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"cups: Printer support", "cups"},
		{"libnotify", "libnotify"},
		{"", ""},
	}
	for _, c := range cases {
		if got := OptDepName(c.in); got != c.want {
			t.Errorf("OptDepName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSplitSourceEntry(t *testing.T) {
	cases := []struct{ entry, name, url string }{
		// name::url form
		{"foo.tar.gz::https://example.com/v1.tar.gz", "foo.tar.gz", "https://example.com/v1.tar.gz"},
		// bare URL: filename derived from the path, fragment stripped
		{"https://example.com/a/v1.2.3.tar.gz", "v1.2.3.tar.gz", "https://example.com/a/v1.2.3.tar.gz"},
		{"git+https://example.com/repo.git#tag=v1", "repo.git", "git+https://example.com/repo.git#tag=v1"},
		// a local file shipped alongside the PKGBUILD
		{"something.patch", "something.patch", "something.patch"},
	}
	for _, c := range cases {
		name, url := SplitSourceEntry(c.entry)
		if name != c.name || url != c.url {
			t.Errorf("SplitSourceEntry(%q) = (%q, %q), want (%q, %q)", c.entry, name, url, c.name, c.url)
		}
	}
}

func TestIsDevel(t *testing.T) {
	if !(&PKGBUILD{Source: []string{"git+https://x/y.git"}}).IsDevel() {
		t.Error("git+ source should be devel")
	}
	if (&PKGBUILD{Source: []string{"https://x/y.tar.gz"}}).IsDevel() {
		t.Error("plain tarball should not be devel")
	}
	if (&PKGBUILD{}).IsDevel() {
		t.Error("no sources should not be devel")
	}
}

func TestSummarizeHandlesArchOverridesAndOddities(t *testing.T) {
	// Values built from shell variables can't be resolved by a text scrape;
	// they should pass through as-written rather than breaking parsing.
	src := `pkgname=foo
pkgver=2.0
pkgrel=3
pkgdesc="A thing"
url=https://example.com
license=('GPL-3.0-or-later' 'MIT')
depends=('a' 'b>=1.0'
         'c')
source=("$pkgname-$pkgver.tar.gz::https://dl.example.com/f.tar.gz")
`
	s := Summarize([]byte(src))
	if s.Name != "foo" || s.Version != "2.0-3" {
		t.Errorf("Name/Version = %q/%q", s.Name, s.Version)
	}
	if s.Description != "A thing" {
		t.Errorf("Description = %q", s.Description)
	}
	if s.License != "GPL-3.0-or-later, MIT" {
		t.Errorf("License = %q, want both entries", s.License)
	}
	// A multi-line depends array must be captured in full.
	if len(s.Depends) != 3 {
		t.Errorf("Depends = %v, want 3 entries across the wrapped array", s.Depends)
	}
	if hosts := s.SourceHosts(); len(hosts) != 1 || hosts[0] != "dl.example.com" {
		t.Errorf("SourceHosts = %v", hosts)
	}
}

func TestSummarizeEmptyInput(t *testing.T) {
	s := Summarize(nil)
	if s.Name != "" || s.HasInstall || len(s.Depends) != 0 || len(s.SourceHosts()) != 0 {
		t.Errorf("empty input produced %+v", s)
	}
}

// Regression: a PKGBUILD with epoch= must record the same version string
// the AUR reports ("1:1.2.3-1"). Without the epoch, every comparison
// against the AUR looks like an upgrade and the package is rebuilt on
// every single `vpa update`.
func TestFullVersionIncludesEpoch(t *testing.T) {
	cases := []struct {
		pb   PKGBUILD
		want string
	}{
		{PKGBUILD{Ver: "1.92.144", Rel: "1", Epoch: "1"}, "1:1.92.144-1"},
		{PKGBUILD{Ver: "1.2.3", Rel: "2"}, "1.2.3-2"},
		{PKGBUILD{Ver: "1.2.3", Rel: "2", Epoch: "0"}, "1.2.3-2"},
		{PKGBUILD{Ver: "1.2.3", Rel: "2", Epoch: "3"}, "3:1.2.3-2"},
	}
	for _, c := range cases {
		if got := c.pb.FullVersion(); got != c.want {
			t.Errorf("FullVersion(%+v) = %q, want %q", c.pb, got, c.want)
		}
	}
}

func TestSummarizeIncludesEpoch(t *testing.T) {
	s := Summarize([]byte("pkgname=x\npkgver=1.92.144\npkgrel=1\nepoch=1\n"))
	if s.Version != "1:1.92.144-1" {
		t.Errorf("Summary.Version = %q, want 1:1.92.144-1", s.Version)
	}
	s = Summarize([]byte("pkgname=x\npkgver=1.0\npkgrel=1\n"))
	if s.Version != "1.0-1" {
		t.Errorf("Summary.Version = %q, want 1.0-1 (no epoch)", s.Version)
	}
}
