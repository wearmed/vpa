package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileIsEmpty(t *testing.T) {
	m, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("Load of a missing manifest should succeed, got %v", err)
	}
	if !m.Empty() {
		t.Error("a missing manifest should be empty")
	}
}

func TestLoadEmptyFileIsEmpty(t *testing.T) {
	p := filepath.Join(t.TempDir(), "installed.json")
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Load(p)
	if err != nil {
		t.Fatalf("Load of an empty file should succeed, got %v", err)
	}
	if !m.Empty() {
		t.Error("an empty file should give an empty manifest")
	}
}

func TestSetGetRemoveRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "installed.json")
	m, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	m.Set("pipes.sh", "1.3.0-1", "")
	m.Set("foo-git", "r1-1", "abc123")
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}

	m2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	e, ok := m2.Get("pipes.sh")
	if !ok || e.Version != "1.3.0-1" || e.Commit != "" {
		t.Errorf("pipes.sh round-trip = %+v ok=%v", e, ok)
	}
	e, ok = m2.Get("foo-git")
	if !ok || e.Commit != "abc123" {
		t.Errorf("devel commit not persisted: %+v", e)
	}

	m2.Remove("pipes.sh")
	if _, ok := m2.Get("pipes.sh"); ok {
		t.Error("Remove did not delete the entry")
	}
	if err := m2.Save(); err != nil {
		t.Fatal(err)
	}
	m3, _ := Load(p)
	if _, ok := m3.Get("pipes.sh"); ok {
		t.Error("removal did not persist")
	}
	if _, ok := m3.Get("foo-git"); !ok {
		t.Error("Remove deleted the wrong entry")
	}
}

func TestSetOverwrites(t *testing.T) {
	m, _ := Load(filepath.Join(t.TempDir(), "m.json"))
	m.Set("a", "1", "")
	m.Set("a", "2", "sha")
	e, _ := m.Get("a")
	if e.Version != "2" || e.Commit != "sha" {
		t.Errorf("Set should overwrite, got %+v", e)
	}
	if len(m.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(m.Entries))
	}
}
