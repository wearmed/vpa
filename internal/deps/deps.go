// Package deps classifies and resolves PKGBUILD dependencies against Void's
// xbps repos, falling back to the AUR, using depmap.conf to bridge Arch/Void
// package-name drift.
package deps

import (
	"bufio"
	_ "embed"
	"bytes"
	"fmt"
	"os"
	"strings"
	"sync"

	"vpa/internal/archrepo"
	"vpa/internal/aurapi"
	"vpa/internal/pkgbuild"
	"vpa/internal/ui"
	"vpa/internal/xbpsutil"
)

//go:embed depmap.conf
var defaultDepmap []byte

type Class int

const (
	Installed Class = iota
	Available
	SonameMatch // resolved by matching Arch's real shared-library sonames to a Void package providing the same ones, not by name
	AUR
	Unresolved
)

type Classification struct {
	Class        Class
	ResolvedName string
	Reason       string
	AURBase      string
}

var (
	depmap     map[string]string
	depmapOnce sync.Once
)

// loadDepmap parses defaultDepmap first, then userDepmapPath if present, so
// user entries win and the last line for a name in either file wins --
// matching the old bash version's "check user file first" semantics via a
// single merged map instead of grepping files on every lookup. Guarded by
// sync.Once: Classify (its only caller, via DepmapLookup) isn't currently
// invoked concurrently anywhere, but a bare nil-check-then-init race is a
// trivial footgun to leave lying around for whenever that changes.
func loadDepmap(userDepmapPath string) {
	depmapOnce.Do(func() {
		depmap = make(map[string]string)
		parseInto(depmap, defaultDepmap)
		if data, err := os.ReadFile(userDepmapPath); err == nil {
			parseInto(depmap, data)
		}
	})
}

func parseInto(m map[string]string, data []byte) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		m[name] = val
	}
}

// DepmapLookup returns the mapped Void name, the original if unmapped, or
// "-" if explicitly marked as having no Void equivalent.
func DepmapLookup(userDepmapPath, name string) string {
	// sync.Once inside loadDepmap makes this safe to call unconditionally
	// even if Classify is ever called concurrently: every caller is
	// guaranteed to see the fully-populated map once this returns.
	loadDepmap(userDepmapPath)
	if v, ok := depmap[name]; ok {
		return v
	}
	return name
}

// Resolver walks a package's dependency tree, building a dependency-first
// build plan with cycle detection, and memoizes Classify results (the same
// dep gets classified multiple times per package, and repeats across
// packages in one run).
type Resolver struct {
	UserDepmap string
	RepoDir    string

	PlanOrder  []string
	Unresolved []string
	// Edges maps a pkgbase to the direct AUR-only pkgbases it depends on --
	// used to group PlanOrder into dependency tiers for safe parallel builds
	// (see Tiers).
	Edges map[string][]string

	cache     map[string]Classification
	planSeen  map[string]bool
	planStack []string

	sonameIndex     map[string]string
	sonameIndexOnce sync.Once
}

func NewResolver(userDepmap, repoDir string) *Resolver {
	return &Resolver{
		UserDepmap: userDepmap,
		RepoDir:    repoDir,
		cache:      make(map[string]Classification),
		planSeen:   make(map[string]bool),
		Edges:      make(map[string][]string),
	}
}

// sonames returns the Void soname->pkgname reverse index, built at most
// once per Resolver (lazily, since it's only needed once a dependency
// actually falls through to the soname-matching fallback).
func (r *Resolver) sonames() map[string]string {
	r.sonameIndexOnce.Do(func() {
		idx, err := xbpsutil.SonameProviders(r.RepoDir)
		if err != nil {
			ui.Warn("couldn't build the shared-library index for cross-distro dependency matching: %v", err)
			idx = map[string]string{}
		}
		r.sonameIndex = idx
	})
	return r.sonameIndex
}

// Classify classifies a raw Arch dependency string against installed
// packages, Void/local repos, the AUR, and -- as a last resort -- Void
// packages that ship the same shared libraries as the real Arch package
// (see archrepo), memoized per raw string.
func (r *Resolver) Classify(raw string) Classification {
	if c, ok := r.cache[raw]; ok {
		return c
	}

	bare := pkgbuild.StripVersion(raw)
	mapped := DepmapLookup(r.UserDepmap, bare)

	var c Classification
	switch {
	case mapped == "-":
		c = Classification{Class: Unresolved, ResolvedName: bare, Reason: "no Void equivalent (per depmap.conf)"}
	case xbpsutil.IsInstalled(mapped):
		c = Classification{Class: Installed, ResolvedName: mapped}
	case xbpsutil.IsAvailable(mapped, r.RepoDir):
		c = Classification{Class: Available, ResolvedName: mapped}
	default:
		base, _ := aurapi.PackageBase(bare)
		if base != "" {
			c = Classification{Class: AUR, ResolvedName: bare, AURBase: base}
			break
		}
		if voidName, ok := r.matchBySoname(bare); ok {
			c = Classification{Class: SonameMatch, ResolvedName: voidName}
			ui.Info("'%s' has no Void package by that name, but '%s' provides the same shared libraries -- using it", bare, voidName)
			break
		}
		c = Classification{Class: Unresolved, ResolvedName: bare, Reason: "not installed, not in Void repos, not found in AUR"}
	}

	r.cache[raw] = c
	return c
}

// matchBySoname looks up name as a real Arch package, and checks whether
// any Void package provides one of the same shared libraries it ships.
func (r *Resolver) matchBySoname(name string) (string, bool) {
	sonames := archrepo.Sonames(name)
	if len(sonames) == 0 {
		return "", false
	}
	index := r.sonames()
	for _, so := range sonames {
		if voidName, ok := index[so]; ok {
			return voidName, true
		}
	}
	return "", false
}

// ReviewFunc reviews+loads a freshly cloned/updated AUR package base's
// PKGBUILD, returning the parsed result. Implemented by the caller (it owns
// the "show + confirm before sourcing" UI policy).
type ReviewFunc func(pkgbase, dir string) (*pkgbuild.PKGBUILD, error)

// CloneFunc clones/updates an AUR package base into a directory.
type CloneFunc func(pkgbase, dir string) error

// Resolve walks pkgbase's dependencies (post-order => PlanOrder ends up in
// valid dependency-first build order), recursing into AUR-only deps first.
func (r *Resolver) Resolve(pkgbase, gitdir, buildDir string, clone CloneFunc, review ReviewFunc) error {
	for _, s := range r.planStack {
		if s == pkgbase {
			return fmt.Errorf("dependency cycle detected: %s -> %s", strings.Join(r.planStack, " -> "), pkgbase)
		}
	}
	if r.planSeen[pkgbase] {
		return nil
	}
	r.planStack = append(r.planStack, pkgbase)

	pb, err := review(pkgbase, gitdir)
	if err != nil {
		return err
	}

	combined := append(append([]string{}, pb.Depends...), pb.MakeDepends...)
	for _, raw := range combined {
		c := r.Classify(raw)
		switch c.Class {
		case Installed, Available, SonameMatch:
			// nothing to do
		case AUR:
			r.Edges[pkgbase] = append(r.Edges[pkgbase], c.AURBase)
			subdir := buildDir + "/" + c.AURBase + "/git"
			if err := clone(c.AURBase, subdir); err != nil {
				return err
			}
			if err := r.Resolve(c.AURBase, subdir, buildDir, clone, review); err != nil {
				return err
			}
		case Unresolved:
			r.Unresolved = append(r.Unresolved, fmt.Sprintf("%s needs '%s' (%s)", pkgbase, raw, c.Reason))
		}
	}

	r.PlanOrder = append(r.PlanOrder, pkgbase)
	r.planSeen[pkgbase] = true
	r.planStack = r.planStack[:len(r.planStack)-1]
	return nil
}

// Tiers groups PlanOrder into dependency levels: tier 0 has no unbuilt AUR
// deps, tier 1's deps are all satisfied by tier 0, and so on. Packages
// within a tier have no dependency relationship to each other, so they can
// build concurrently; each tier must be installed before the next tier's
// builds start, since a later tier's build() may need an earlier tier's
// AUR-only package actually present on disk (a real compile-time
// dependency, not just a runtime one).
func (r *Resolver) Tiers() [][]string {
	level := make(map[string]int, len(r.PlanOrder))
	for _, pb := range r.PlanOrder {
		best := 0
		for _, dep := range r.Edges[pb] {
			if level[dep]+1 > best {
				best = level[dep] + 1
			}
		}
		level[pb] = best
	}

	var tiers [][]string
	for _, pb := range r.PlanOrder {
		l := level[pb]
		for len(tiers) <= l {
			tiers = append(tiers, nil)
		}
		tiers[l] = append(tiers[l], pb)
	}
	return tiers
}

// RuntimeDepsString builds the resolved runtime dependency pkgpattern
// string for xbps-create -D, from Depends only (never MakeDepends, which
// are build-time only). xbps pkgpatterns require a version operator, so an
// unversioned dep gets ">=0" appended -- void-packages' own convention.
func (r *Resolver) RuntimeDepsString(pb *pkgbuild.PKGBUILD) string {
	var out []string
	for _, raw := range pb.Depends {
		c := r.Classify(raw)
		if c.Class == Unresolved {
			continue
		}
		ver := pkgbuild.DepVersion(raw)
		if ver == "" {
			ver = ">=0"
		}
		out = append(out, c.ResolvedName+ver)
	}
	return strings.Join(out, " ")
}
