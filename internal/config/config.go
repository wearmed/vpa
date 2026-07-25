// Package config resolves vur's on-disk paths and loads its config file.
package config

import (
	"bufio"
	"os"
	"path/filepath"
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

	NoConfirm    bool
	EditPKGBUILD bool
	Devel        bool
	CleanAfter   bool
	Editor       string
}

const (
	AURRPC = "https://aur.archlinux.org/rpc/v5"
	AURGit = "https://aur.archlinux.org"
)

// Load resolves all paths and reads ~/.config/vur/vur.conf if present.
func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	cacheDir := os.Getenv("VUR_CACHE")
	if cacheDir == "" {
		cacheDir = filepath.Join(home, ".cache", "vur")
	}
	configDir := filepath.Join(home, ".config", "vur")

	c := &Config{
		CacheDir:     cacheDir,
		BuildDir:     filepath.Join(cacheDir, "build"),
		RepoDir:      filepath.Join(cacheDir, "repo"),
		ManifestFile: filepath.Join(cacheDir, "installed.json"),
		ReviewedDir:  filepath.Join(cacheDir, "reviewed"),
		ConfigDir:    configDir,
		ConfigFile:   filepath.Join(configDir, "vur.conf"),
		UserDepmap:   filepath.Join(configDir, "depmap.conf"),
		Editor:       firstNonEmpty(os.Getenv("EDITOR"), os.Getenv("VISUAL"), "vi"),
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
		case "EDIT_PKGBUILD":
			c.EditPKGBUILD = val == "1"
		case "DEVEL":
			c.Devel = val == "1"
		case "CLEAN_AFTER":
			c.CleanAfter = val == "1"
		case "EDITOR":
			c.Editor = val
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
