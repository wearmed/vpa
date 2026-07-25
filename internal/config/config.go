// Package config resolves vpa's on-disk paths and loads its config file.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	CacheDir     string
	BuildDir     string
	RepoDir      string
	ManifestFile string
	ReviewedDir  string
	ConfigDir    string
	ConfigFile   string
	UserDepmap   string

	NoConfirm     bool
	TrustAUR      bool
	PreferFlatpak bool
	EditPKGBUILD  bool
	Devel         bool
	CleanAfter    bool
	Editor        string
	Parallel      int
}

const (
	AURRPC = "https://aur.archlinux.org/rpc/v5"
	AURGit = "https://aur.archlinux.org"
)

// Load resolves all paths and reads ~/.config/vpa/vpa.conf if present.
func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	cacheDir := os.Getenv("VPA_CACHE")
	if cacheDir == "" {
		cacheDir = filepath.Join(home, ".cache", "vpa")
	}
	configDir := filepath.Join(home, ".config", "vpa")

	// The project was renamed from vur to vpa; carry an existing install's
	// state (tracked packages, source cache, config) across on first run so
	// the rename isn't a silent data loss. Safe to delete once nobody's
	// running a pre-rename version anymore.
	migrateLegacyDir(filepath.Join(home, ".cache", "vur"), cacheDir)
	migrateLegacyDir(filepath.Join(home, ".config", "vur"), configDir)
	migrateLegacyFile(filepath.Join(configDir, "vur.conf"), filepath.Join(configDir, "vpa.conf"))

	c := &Config{
		CacheDir:     cacheDir,
		BuildDir:     filepath.Join(cacheDir, "build"),
		RepoDir:      filepath.Join(cacheDir, "repo"),
		ManifestFile: filepath.Join(cacheDir, "installed.json"),
		ReviewedDir:  filepath.Join(cacheDir, "reviewed"),
		ConfigDir:    configDir,
		ConfigFile:   filepath.Join(configDir, "vpa.conf"),
		UserDepmap:   filepath.Join(configDir, "depmap.conf"),
		Editor:       firstNonEmpty(os.Getenv("EDITOR"), os.Getenv("VISUAL"), "vi"),
		Parallel:     4,
		// Routine confirmations default to yes; toggle with
		// `vpa --assumeno` (or NOCONFIRM=0 in the config file).
		NoConfirm: true,
	}

	for _, d := range []string{c.BuildDir, c.RepoDir, c.ReviewedDir, c.ConfigDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}

	if err := c.loadFile(); err != nil {
		return nil, err
	}
	return c, nil
}

// loadFile parses simple KEY=value lines from ConfigFile, same trust level
// as a personal shell rc file: no expressions, just literal values.
func (c *Config) loadFile() error {
	f, err := os.Open(c.ConfigFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		switch key {
		case "NOCONFIRM":
			c.NoConfirm = val == "1"
		case "TRUST_AUR":
			c.TrustAUR = val == "1"
		case "PREFER_FLATPAK":
			c.PreferFlatpak = val == "1"
		case "EDIT_PKGBUILD":
			c.EditPKGBUILD = val == "1"
		case "DEVEL":
			c.Devel = val == "1"
		case "CLEAN_AFTER":
			c.CleanAfter = val == "1"
		case "EDITOR":
			c.Editor = val
		case "PARALLEL_DOWNLOADS":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				c.Parallel = n
			}
		}
	}
	return sc.Err()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// migrateLegacyDir moves an old vur-era directory to its vpa location, but
// only when the old one exists and the new one doesn't -- so it never
// clobbers real vpa state, and does nothing at all on a fresh install or
// on every run after the first.
func migrateLegacyDir(old, new string) {
	if old == new {
		return
	}
	if _, err := os.Stat(new); err == nil {
		return
	}
	if fi, err := os.Stat(old); err != nil || !fi.IsDir() {
		return
	}
	os.MkdirAll(filepath.Dir(new), 0o755)
	if err := os.Rename(old, new); err == nil {
		fmt.Fprintf(os.Stderr, ":: migrated %s -> %s (project renamed vur -> vpa)\n", old, new)
	}
}

func migrateLegacyFile(old, new string) {
	if old == new {
		return
	}
	if _, err := os.Stat(new); err == nil {
		return
	}
	if fi, err := os.Stat(old); err != nil || fi.IsDir() {
		return
	}
	if err := os.Rename(old, new); err == nil {
		fmt.Fprintf(os.Stderr, ":: migrated %s -> %s (project renamed vur -> vpa)\n", old, new)
	}
}

// SetOption updates (or adds) a KEY=value line in the config file,
// preserving everything else in it including comments, so the file stays
// something a person can still read and edit by hand.
func SetOption(path, key, value string) error {
	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	}

	replaced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if k, _, ok := strings.Cut(trimmed, "="); ok && strings.TrimSpace(k) == key {
			lines[i] = key + "=" + value
			replaced = true
			break
		}
	}
	if !replaced {
		lines = append(lines, key+"="+value)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}
