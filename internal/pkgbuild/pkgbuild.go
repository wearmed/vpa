// Package pkgbuild sources a PKGBUILD via bash (the only reliable parser
// for it, since it's arbitrary bash) and exposes its variables as a struct.
package pkgbuild

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"vpa/internal/gitutil"
	"vpa/internal/xbpsutil"
)

var (
	archOnce  sync.Once
	archValue string
)

// currentArch is cached for the process lifetime -- it can't change mid-run.
func currentArch() string {
	archOnce.Do(func() {
		archValue, _ = xbpsutil.Arch()
	})
	return archValue
}

//go:embed extract.sh
var extractScript []byte

type PKGBUILD struct {
	Names        []string
	Base         string
	Ver          string
	Rel          string
	Desc         string
	URL          string
	Install      string
	Arch         []string
	License      []string
	Depends      []string
	MakeDepends  []string
	CheckDepends []string
	OptDepends   []string
	Provides     []string
	Conflicts    []string
	Replaces     []string
	Source       []string
	Sha256sums   []string
	Sha512sums   []string
	B2sums       []string
	Md5sums      []string
	NoExtract    []string
}

// Load sources dir/PKGBUILD and parses its variables.
func Load(dir string) (*PKGBUILD, error) {
	if _, err := os.Stat(filepath.Join(dir, "PKGBUILD")); err != nil {
		return nil, fmt.Errorf("no PKGBUILD found in %s", dir)
	}

	tmp, err := os.CreateTemp("", "vpa-extract-*.sh")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(extractScript); err != nil {
		tmp.Close()
		return nil, err
	}
	tmp.Close()

	cmd := exec.Command("bash", tmp.Name(), dir, currentArch())
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to parse PKGBUILD in %s: %w", dir, err)
	}

	pb := &PKGBUILD{Rel: "1"}
	records := bytes.Split(out.Bytes(), []byte{0})

	var curArray *[]string
	arrayFields := map[string]*[]string{
		"arch": &pb.Arch, "license": &pb.License, "depends": &pb.Depends,
		"makedepends": &pb.MakeDepends, "checkdepends": &pb.CheckDepends,
		"optdepends": &pb.OptDepends, "provides": &pb.Provides,
		"conflicts": &pb.Conflicts, "replaces": &pb.Replaces,
		"source": &pb.Source, "sha256sums": &pb.Sha256sums,
		"sha512sums": &pb.Sha512sums, "b2sums": &pb.B2sums,
		"md5sums": &pb.Md5sums, "noextract": &pb.NoExtract,
	}

	for _, rec := range records {
		if len(rec) == 0 {
			continue
		}
		s := string(rec)
		switch {
		case strings.HasPrefix(s, "S\t"):
			curArray = nil
			parts := strings.SplitN(s, "\t", 3)
			name, val := parts[1], ""
			if len(parts) == 3 {
				val = parts[2]
			}
			switch name {
			case "pkgname":
				pb.Names = []string{val}
			case "pkgbase":
				pb.Base = val
			case "pkgver":
				pb.Ver = val
			case "pkgrel":
				if val != "" {
					pb.Rel = val
				}
			case "pkgdesc":
				pb.Desc = val
			case "url":
				pb.URL = val
			case "install":
				pb.Install = val
			}
		case strings.HasPrefix(s, "A\t"):
			name := s[2:]
			if name == "pkgname" {
				pb.Names = nil
				curArray = &pb.Names
				continue
			}
			if f, ok := arrayFields[name]; ok {
				*f = nil
				curArray = f
			} else {
				curArray = nil
			}
		case strings.HasPrefix(s, "X\t"):
			// Arch-specific override (e.g. source_x86_64): appends to the
			// existing base array (source, depends, ...) rather than
			// resetting it, matching makepkg's own arch-override semantics.
			name := s[2:]
			if f, ok := arrayFields[name]; ok {
				curArray = f
			} else {
				curArray = nil
			}
		case strings.HasPrefix(s, "I\t"):
			if curArray != nil {
				*curArray = append(*curArray, s[2:])
			}
		}
	}

	if len(pb.Names) == 0 || pb.Names[0] == "" {
		return nil, fmt.Errorf("PKGBUILD in %s defines no pkgname", dir)
	}
	if pb.Base == "" {
		pb.Base = pb.Names[0]
	}
	return pb, nil
}

// IsDevel reports whether any Source entry is a git+ URL (VCS package).
func (pb *PKGBUILD) IsDevel() bool {
	for _, s := range pb.Source {
		if strings.Contains(s, "git+") {
			return true
		}
	}
	return false
}

// DevelLatestCommit returns the HEAD of the first git+ source's remote via
// a cheap ls-remote (no clone needed). Empty if not a devel package.
func (pb *PKGBUILD) DevelLatestCommit() string {
	for _, s := range pb.Source {
		_, u := SplitSourceEntry(s)
		if !strings.HasPrefix(u, "git+") {
			continue
		}
		real := strings.TrimPrefix(u, "git+")
		base, _, _ := strings.Cut(real, "#")
		return gitutil.LsRemoteHead(base)
	}
	return ""
}

// SplitSourceEntry splits a `name::url` PKGBUILD source entry, or derives
// the filename from the URL/local-path if there's no `::`.
func SplitSourceEntry(entry string) (name, url string) {
	if n, u, ok := strings.Cut(entry, "::"); ok {
		return n, u
	}
	url = entry
	if strings.HasPrefix(url, "git+") || strings.Contains(url, "://") {
		base, _, _ := strings.Cut(url, "#")
		return filepath.Base(base), url
	}
	return url, url
}

// StripVersion strips an Arch dependency string's version-constraint
// suffix (>=, <=, =, <, >), returning the bare package name.
func StripVersion(dep string) string {
	for i, r := range dep {
		if r == '<' || r == '>' || r == '=' {
			return dep[:i]
		}
	}
	return dep
}

// DepVersion returns the version-constraint suffix (e.g. ">=4.0.0"), or empty.
func DepVersion(dep string) string {
	bare := StripVersion(dep)
	return dep[len(bare):]
}

// OptDepName returns the pkgname part of an optdepends entry like "foo: needed for bar".
func OptDepName(opt string) string {
	name, _, _ := strings.Cut(opt, ":")
	return name
}
