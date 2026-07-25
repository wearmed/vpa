package foreignpkg

import "testing"

func TestDetect(t *testing.T) {
	cases := []struct {
		in   string
		want Format
	}{
		{"hello_2.8-4_amd64.deb", Debian},
		{"hello-2.12.3-1.fc44.x86_64.rpm", RPM},
		{"licenses-20240728-1-any.pkg.tar.zst", Arch},
		{"foo-1.0-1-x86_64.pkg.tar.xz", Arch},
		{"foo-1.0-1-x86_64.pkg.tar.gz", Arch},
		{"https://example.com/a/hello_1.0_amd64.DEB", Debian}, // case-insensitive
		{"pipes.sh", Unknown},
		{"firefox", Unknown},
		{"foo.xbps", Unknown}, // native, handled elsewhere
		{"", Unknown},
	}
	for _, c := range cases {
		if got := Detect(c.in); got != c.want {
			t.Errorf("Detect(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestSplitVerRel(t *testing.T) {
	cases := []struct{ in, ver, rel string }{
		{"2.8-4", "2.8", "4"},
		{"1.2.3-1ubuntu2", "1.2.3", "1ubuntu2"},
		{"2.12.3-1.fc44", "2.12.3", "1.fc44"},
		{"20240728-1", "20240728", "1"},
		{"1.0", "1.0", "1"}, // no dash: synthetic release
		{"", "", "1"},
	}
	for _, c := range cases {
		ver, rel := splitVerRel(c.in)
		if ver != c.ver || rel != c.rel {
			t.Errorf("splitVerRel(%q) = (%q, %q), want (%q, %q)", c.in, ver, rel, c.ver, c.rel)
		}
	}
}

func TestParseDebControl(t *testing.T) {
	control := `Package: hello
Version: 2.8-4
Architecture: amd64
Maintainer: Ubuntu Developers <x@y.z>
Installed-Size: 108
Depends: libc6 (>= 2.14), dpkg (>= 1.15.4) | install-info
Section: devel
Homepage: http://www.gnu.org/software/hello/
Description: The classic greeting, and a good example
 The GNU hello program produces a familiar, friendly greeting.
 .
 Continuation lines must not be parsed as fields.
`
	m := parseDebControl(control)
	if m.Name != "hello" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.Version != "2.8" || m.Release != "4" {
		t.Errorf("Version/Release = %q/%q", m.Version, m.Release)
	}
	if m.URL != "http://www.gnu.org/software/hello/" {
		t.Errorf("URL = %q", m.URL)
	}
	if m.Desc != "The classic greeting, and a good example" {
		t.Errorf("Description = %q (continuation lines must not leak in)", m.Desc)
	}
	if len(m.Depends) != 2 {
		t.Errorf("Depends = %v, want 2 comma-separated entries", m.Depends)
	}
}

func TestRPMFilenamePattern(t *testing.T) {
	cases := []struct{ file, name, ver, rel string }{
		{"hello-2.12.3-1.fc44.x86_64.rpm", "hello", "2.12.3", "1.fc44"},
		{"my-cool-pkg-1.0-2.el9.noarch.rpm", "my-cool-pkg", "1.0", "2.el9"},
	}
	for _, c := range cases {
		m := rpmFilenameRe.FindStringSubmatch(c.file)
		if m == nil {
			t.Errorf("%s did not match the RPM filename pattern", c.file)
			continue
		}
		if m[1] != c.name || m[2] != c.ver || m[3] != c.rel {
			t.Errorf("%s -> (%q, %q, %q), want (%q, %q, %q)", c.file, m[1], m[2], m[3], c.name, c.ver, c.rel)
		}
	}
	if rpmFilenameRe.MatchString("not-an-rpm.tar.gz") {
		t.Error("non-RPM filename matched the RPM pattern")
	}
}

func TestFetchRejectsMissingLocalFile(t *testing.T) {
	_, cleanup, err := Fetch("/definitely/does/not/exist.deb")
	defer cleanup()
	if err == nil {
		t.Error("Fetch of a nonexistent local file should fail")
	}
}
