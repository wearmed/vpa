package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeVersRel(t *testing.T) {
	// xbps-create requires name-version_revision with a plain-integer
	// revision; RPM/deb releases like "1.fc44" are rejected outright, so a
	// non-numeric release folds into the version with a synthetic revision.
	cases := []struct{ ver, rel, wantVer, wantRel string }{
		{"2.8", "4", "2.8", "4"},
		{"2.12.3", "1.fc44", "2.12.3.1.fc44", "1"},
		{"1.2.3", "1ubuntu2", "1.2.3.1ubuntu2", "1"},
		{"20240728", "1", "20240728", "1"},
		{"1.0", "", "1.0", "1"},
		// Characters xbps won't accept in a version must be normalised away.
		{"1.0~beta+1", "1", "1.0.beta.1", "1"},
	}
	for _, c := range cases {
		gotVer, gotRel := sanitizeVersRel(c.ver, c.rel)
		if gotVer != c.wantVer || gotRel != c.wantRel {
			t.Errorf("sanitizeVersRel(%q,%q) = (%q,%q), want (%q,%q)",
				c.ver, c.rel, gotVer, gotRel, c.wantVer, c.wantRel)
		}
	}
}

func TestSanitizeName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"licenses-20240728-1-any.pkg.tar.zst", "licenses-20240728-1-any"},
		{"hello_2.8-4_amd64.deb", "hello_2.8-4_amd64"},
		{"hello-2.12.3-1.fc44.x86_64.rpm", "hello-2.12.3-1.fc44.x86_64"},
		{"plain", "plain"},
	}
	for _, c := range cases {
		if got := sanitizeName(c.in); got != c.want {
			t.Errorf("sanitizeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsXbpsFileArg(t *testing.T) {
	for _, in := range []string{"foo.xbps", "/tmp/a-1.0_1.x86_64.xbps", "HTTP://x/Y.XBPS"} {
		if !isXbpsFileArg(in) {
			t.Errorf("isXbpsFileArg(%q) = false, want true", in)
		}
	}
	for _, in := range []string{"firefox", "foo.deb", "foo.xbps.sig", ""} {
		if isXbpsFileArg(in) {
			t.Errorf("isXbpsFileArg(%q) = true, want false", in)
		}
	}
}

func TestIsForeignPkgArg(t *testing.T) {
	for _, in := range []string{"a.deb", "b.rpm", "c.pkg.tar.zst"} {
		if !isForeignPkgArg(in) {
			t.Errorf("isForeignPkgArg(%q) = false, want true", in)
		}
	}
	for _, in := range []string{"firefox", "pipes.sh", "a.xbps"} {
		if isForeignPkgArg(in) {
			t.Errorf("isForeignPkgArg(%q) = true, want false", in)
		}
	}
}

// canonicalCommand is the single source of truth shared by dispatch and
// help lookup; if an alias resolves here it must also be dispatchable.
func TestCanonicalCommand(t *testing.T) {
	cases := map[string]string{
		"install": "install", "i": "install",
		"remove": "remove", "rm": "remove",
		"search": "search", "s": "search",
		"info":   "info",
		"update": "update", "up": "update", "upgrade": "update",
		"list": "list", "ls": "list",
		"filelist": "filelist", "fl": "filelist", "files": "filelist",
		"whatprovides": "whatprovides", "wp": "whatprovides", "owns": "whatprovides",
		"reverse": "reverse", "rv": "reverse", "revdeps": "reverse",
		"listrepos": "listrepos", "lr": "listrepos", "repos": "listrepos",
		"cleanup": "cleanup", "cl": "cleanup", "clean": "cleanup",
		"help": "help", "h": "help", "?": "help",
		"devinstall": "devinstall", "di": "devinstall",
		"searchfile": "searchfile", "sf": "searchfile",
		"autoremove": "autoremove", "ar": "autoremove",
		"": "", "nonsense": "", "instal": "",
	}
	for in, want := range cases {
		if got := canonicalCommand(in); got != want {
			t.Errorf("canonicalCommand(%q) = %q, want %q", in, got, want)
		}
	}
}

// Every command the user can reach must have its own help text, or
// `vpa <cmd>` with no args (and `vpa help <cmd>`) silently shows nothing.
func TestEveryCommandHasHelp(t *testing.T) {
	commands := []string{
		"search", "info", "install", "devinstall", "forceinstall",
		"remove", "removerecursive", "update", "sync", "list",
		"filelist", "whatprovides", "searchfile", "deps", "reverse",
		"orphans", "autoremove", "reconfigure", "listrepos", "addrepo",
		"listalternatives", "setalternative", "cleanup", "hold", "unhold",
	}
	for _, c := range commands {
		if _, ok := commandHelp[c]; !ok {
			t.Errorf("command %q has no help entry", c)
		}
	}
}

// Conversely, help entries must correspond to real commands.
func TestNoOrphanHelpEntries(t *testing.T) {
	for name := range commandHelp {
		if canonicalCommand(name) != name {
			t.Errorf("help entry %q is not a canonical command name", name)
		}
	}
}

// Regression: installing a .xbps that already lives in the repo directory
// used to open it for reading and then truncate it via os.Create, leaving a
// zero-byte package behind and destroying the file.
func TestCopyToRepoSelfCopyDoesNotTruncate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pkg-1.0_1.x86_64.xbps")
	content := []byte("real package bytes")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyToRepo(path, path); err != nil {
		t.Fatalf("self-copy returned an error: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("file content changed: got %q, want %q", got, content)
	}
}

func TestCopyToRepoNormalCopy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.xbps")
	dst := filepath.Join(dir, "sub", "a.xbps")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("payload")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyToRepo(src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("copied content = %q, want %q", got, content)
	}
}
