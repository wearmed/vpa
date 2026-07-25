// Package manifest tracks packages vur has installed, as JSON.
package manifest

import (
	"encoding/json"
	"os"
)

type Entry struct {
	Version string `json:"version"`
	Commit  string `json:"commit,omitempty"`
}

type Manifest struct {
	path    string
	Entries map[string]Entry `json:"entries"`
}

// Load reads path, returning an empty manifest if it doesn't exist yet.
func Load(path string) (*Manifest, error) {
	m := &Manifest{path: path, Entries: make(map[string]Entry)}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(data, m); err != nil {
		return nil, err
	}
	if m.Entries == nil {
		m.Entries = make(map[string]Entry)
	}
	return m, nil
}

// Save atomically writes the manifest back to disk.
func (m *Manifest) Save() error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, m.path)
}

func (m *Manifest) Set(name, version, commit string) {
	m.Entries[name] = Entry{Version: version, Commit: commit}
}

func (m *Manifest) Remove(name string) {
	delete(m.Entries, name)
}

func (m *Manifest) Get(name string) (Entry, bool) {
	e, ok := m.Entries[name]
	return e, ok
}

func (m *Manifest) Empty() bool {
	return len(m.Entries) == 0
}
