package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetOptionCreatesAndUpdates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vpa.conf")

	if err := SetOption(path, "NOCONFIRM", "1"); err != nil {
		t.Fatalf("SetOption on a missing file: %v", err)
	}
	if got := read(t, path); !strings.Contains(got, "NOCONFIRM=1") {
		t.Errorf("after create, file = %q", got)
	}

	// Updating must replace in place, not append a second line.
	if err := SetOption(path, "NOCONFIRM", "0"); err != nil {
		t.Fatal(err)
	}
	got := read(t, path)
	if strings.Count(got, "NOCONFIRM=") != 1 {
		t.Errorf("expected exactly one NOCONFIRM line, got %q", got)
	}
	if !strings.Contains(got, "NOCONFIRM=0") {
		t.Errorf("value not updated: %q", got)
	}
}

func TestSetOptionPreservesCommentsAndOtherKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vpa.conf")
	original := `# vpa configuration
# NOCONFIRM=1        # commented example, must not be treated as the real key
EDITOR=nvim
PARALLEL_DOWNLOADS=8
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetOption(path, "NOCONFIRM", "1"); err != nil {
		t.Fatal(err)
	}

	got := read(t, path)
	for _, want := range []string{
		"# vpa configuration",
		"# NOCONFIRM=1        # commented example",
		"EDITOR=nvim",
		"PARALLEL_DOWNLOADS=8",
		"\nNOCONFIRM=1\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// The commented-out example must not have been rewritten as the real key.
	if strings.Contains(got, "# NOCONFIRM=1\n") && !strings.Contains(got, "commented example") {
		t.Error("a commented line was mistaken for the active setting")
	}
}

func TestLoadFileParsesValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vpa.conf")
	body := `# comment
NOCONFIRM=1
TRUST_AUR=1
CLEAN_AFTER=1
EDIT_PKGBUILD=1
DEVEL=1
EDITOR="code --wait"
PARALLEL_DOWNLOADS=12

BOGUS
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &Config{ConfigFile: path}
	if err := c.loadFile(); err != nil {
		t.Fatalf("loadFile: %v", err)
	}
	if !c.NoConfirm || !c.TrustAUR || !c.CleanAfter || !c.EditPKGBUILD || !c.Devel {
		t.Errorf("boolean options not all set: %+v", c)
	}
	if c.Editor != "code --wait" {
		t.Errorf("Editor = %q, want quotes stripped and arguments kept", c.Editor)
	}
	if c.Parallel != 12 {
		t.Errorf("Parallel = %d, want 12", c.Parallel)
	}
}

func TestLoadFileRejectsBadParallel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vpa.conf")
	if err := os.WriteFile(path, []byte("PARALLEL_DOWNLOADS=0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &Config{ConfigFile: path, Parallel: 4}
	if err := c.loadFile(); err != nil {
		t.Fatal(err)
	}
	if c.Parallel != 4 {
		t.Errorf("Parallel = %d; a non-positive value must not override the default", c.Parallel)
	}
}

func TestLoadFileMissingIsNotAnError(t *testing.T) {
	c := &Config{ConfigFile: filepath.Join(t.TempDir(), "nope.conf")}
	if err := c.loadFile(); err != nil {
		t.Errorf("a missing config file should be fine, got %v", err)
	}
}

func TestMigrateLegacyDir(t *testing.T) {
	base := t.TempDir()
	old := filepath.Join(base, "vur")
	newer := filepath.Join(base, "vpa")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "installed.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	migrateLegacyDir(old, newer)
	if _, err := os.Stat(filepath.Join(newer, "installed.json")); err != nil {
		t.Fatalf("state was not migrated: %v", err)
	}

	// Must never clobber existing new-location state.
	old2 := filepath.Join(base, "vur2")
	new2 := filepath.Join(base, "vpa2")
	os.MkdirAll(old2, 0o755)
	os.MkdirAll(new2, 0o755)
	os.WriteFile(filepath.Join(old2, "x"), []byte("old"), 0o644)
	os.WriteFile(filepath.Join(new2, "x"), []byte("new"), 0o644)
	migrateLegacyDir(old2, new2)
	if got := read(t, filepath.Join(new2, "x")); got != "new" {
		t.Errorf("existing state was overwritten by migration: %q", got)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
