package pkgbuild

import "testing"

const samplePKGBUILD = `# Maintainer: Someone <a@b.c>
pkgname=pipes.sh
pkgver=1.3.0
pkgrel=1
pkgdesc='Animated pipes terminal screensaver'
arch=('any')
url='https://github.com/pipeseroni/pipes.sh'
license=('MIT')
depends=('bash>=4.0.0')
install=
source=("https://github.com/pipeseroni/$pkgname/archive/v$pkgver.tar.gz")

package() {
  cd "$pkgname-$pkgver"
  make DESTDIR="$pkgdir/" PREFIX=/usr install
  install -Dm644 -t "$pkgdir/usr/share/doc/$pkgname" LICENSE
}
`

func TestSummarize(t *testing.T) {
	s := Summarize([]byte(samplePKGBUILD))

	if s.Name != "pipes.sh" {
		t.Errorf("Name = %q, want pipes.sh", s.Name)
	}
	if s.Version != "1.3.0-1" {
		t.Errorf("Version = %q, want 1.3.0-1", s.Version)
	}
	if s.Description != "Animated pipes terminal screensaver" {
		t.Errorf("Description = %q", s.Description)
	}
	if s.License != "MIT" {
		t.Errorf("License = %q, want MIT (quotes and parens stripped)", s.License)
	}
	if len(s.Depends) != 1 || s.Depends[0] != "bash>=4.0.0" {
		t.Errorf("Depends = %v", s.Depends)
	}

	// An empty `install=` must not be reported as shipping an install
	// script. Regression: \s spans newlines in Go, so the value pattern
	// used to capture text from a following line here.
	if s.HasInstall {
		t.Error("HasInstall = true for an empty install=, want false")
	}

	hosts := s.SourceHosts()
	if len(hosts) != 1 || hosts[0] != "github.com" {
		t.Errorf("SourceHosts = %v, want [github.com]", hosts)
	}
}

func TestSummarizeInstallScriptDetected(t *testing.T) {
	s := Summarize([]byte("pkgname=foo\ninstall=foo.install\n"))
	if !s.HasInstall {
		t.Error("HasInstall = false for a real install= value, want true")
	}
}
