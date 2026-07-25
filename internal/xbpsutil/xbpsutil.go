// Package xbpsutil wraps the xbps-* CLI tools vpa needs.
package xbpsutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"vpa/internal/sysutil"
)

// Arch returns the configured xbps architecture (e.g. "x86_64").
func Arch() (string, error) {
	out, err := sysutil.Output("xbps-uhelper", "arch")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// IsInstalled reports whether name is currently installed.
func IsInstalled(name string) bool {
	return sysutil.RunSilent("xbps-query", name) == nil
}

// IsAvailable reports whether name is resolvable from Void repos or repoDir.
func IsAvailable(name, repoDir string) bool {
	return sysutil.RunSilent("xbps-query", "-R", "--repository="+repoDir, name) == nil
}

// SonameProviders builds a reverse index of every shared-library soname
// (e.g. "libXss.so.1") provided by any package in Void's configured repos
// (plus repoDir) to the bare package name that provides it. Used to match
// an Arch dependency to its real Void equivalent by actual ABI rather than
// by name -- a bulk dump + one batched name-parsing call, not one
// subprocess per package, so this stays cheap (~0.2s for ~20k packages on
// a real repo set) enough to run as a same-process fallback.
func SonameProviders(repoDir string) (map[string]string, error) {
	out, err := sysutil.Output("xbps-query", "-Rs", "", "--repository="+repoDir, "--property=shlib-provides")
	if err != nil {
		return nil, fmt.Errorf("failed to list shlib-provides: %w", err)
	}

	type line struct {
		pkgver  string
		sonames []string
	}
	var lines []line
	var pkgvers []string
	for _, raw := range strings.Split(out, "\n") {
		pkgver, rest, ok := strings.Cut(raw, ": ")
		if !ok {
			continue
		}
		rest, _, _ = strings.Cut(rest, " (")
		sonames := strings.Fields(rest)
		if len(sonames) == 0 {
			continue
		}
		lines = append(lines, line{pkgver: pkgver, sonames: sonames})
		pkgvers = append(pkgvers, pkgver)
	}
	if len(lines) == 0 {
		return map[string]string{}, nil
	}

	names, err := sysutil.Output("xbps-uhelper", append([]string{"getpkgname"}, pkgvers...)...)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve package names: %w", err)
	}
	nameList := strings.Split(strings.TrimRight(names, "\n"), "\n")
	if len(nameList) != len(lines) {
		return nil, fmt.Errorf("shlib-provides/getpkgname line count mismatch (%d vs %d)", len(lines), len(nameList))
	}

	index := make(map[string]string, len(lines))
	for i, l := range lines {
		for _, so := range l.sonames {
			if _, exists := index[so]; !exists {
				index[so] = nameList[i]
			}
		}
	}
	return index, nil
}

// Create builds pkgname-pkgver_pkgrel.<arch>.xbps from pkgdir into repoDir.
func Create(pkgname, pkgver, pkgrel, pkgdir, deps, desc, url, license, repoDir string) error {
	sysutil.RequireBin("xbps-create", "xbps")

	if pkgname == "" {
		return fmt.Errorf("create_xbps_pkg: empty pkgname (broken PKGBUILD?)")
	}
	if pkgver == "" {
		return fmt.Errorf("%s: empty pkgver -- likely a -git/-svn/-hg PKGBUILD whose pkgver() vpa doesn't invoke; set pkgver= manually with --edit", pkgname)
	}
	fi, err := os.Stat(pkgdir)
	if err != nil || !fi.IsDir() {
		return fmt.Errorf("%s: pkgdir %s doesn't exist -- package() didn't run", pkgname, pkgdir)
	}
	entries, err := os.ReadDir(pkgdir)
	if err != nil || len(entries) == 0 {
		return fmt.Errorf("%s: package() produced an empty directory -- nothing to package", pkgname)
	}

	arch, err := Arch()
	if err != nil {
		return err
	}
	if desc == "" {
		desc = pkgname
	}
	// compression=none: this repo is a throwaway local staging area (wiped by
	// `vpa clean`), so there's nothing to gain from spending CPU time
	// compressing a package that's about to be immediately re-unpacked by
	// xbps-install -- pure overhead for us.
	args := []string{"-A", arch, "-n", fmt.Sprintf("%s-%s_%s", pkgname, pkgver, pkgrel), "-s", desc, "--compression", "none"}
	if deps != "" {
		args = append(args, "-D", deps)
	}
	if url != "" {
		args = append(args, "-H", url)
	}
	if license != "" {
		args = append(args, "-l", license)
	}
	args = append(args, pkgdir)

	cmd := exec.Command("xbps-create", args...)
	cmd.Dir = repoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("xbps-create failed for %s: %w", pkgname, err)
	}

	outfile := filepath.Join(repoDir, fmt.Sprintf("%s-%s_%s.%s.xbps", pkgname, pkgver, pkgrel, arch))
	if _, err := os.Stat(outfile); err != nil {
		return fmt.Errorf("xbps-create reported success for %s but %s is missing", pkgname, outfile)
	}
	return nil
}

// Rindex (re)builds the repodata index over everything in repoDir.
func Rindex(repoDir string) error {
	sysutil.RequireBin("xbps-rindex", "xbps")
	matches, _ := filepath.Glob(filepath.Join(repoDir, "*.xbps"))
	if len(matches) == 0 {
		return nil
	}
	args := append([]string{"-fa"}, matches...)
	return sysutil.RunInteractive("xbps-rindex", args...)
}

const repoSyncInterval = 6 * time.Hour

// needsSync reports whether the configured Void repos' local index data is
// stale enough to warrant a real network sync. `xbps-install -S` re-fetches
// every configured repo's index (not just our own throwaway local one) on
// every single call, which is the dominant cost of an otherwise-instant
// install if run unconditionally -- a marker file under cacheDir bounds how
// often that actually happens.
func needsSync(cacheDir string) bool {
	marker := filepath.Join(cacheDir, "last-repo-sync")
	fi, err := os.Stat(marker)
	if err != nil {
		return true
	}
	return time.Since(fi.ModTime()) > repoSyncInterval
}

func markSynced(cacheDir string) {
	marker := filepath.Join(cacheDir, "last-repo-sync")
	now := time.Now()
	if err := os.Chtimes(marker, now, now); err != nil {
		f, ferr := os.Create(marker)
		if ferr == nil {
			f.Close()
		}
	}
}

// Install installs pkgs from repoDir via sudo xbps-install, syncing the
// configured Void repos' index only if it looks stale (see needsSync).
func Install(repoDir, cacheDir string, pkgs ...string) error {
	if len(pkgs) == 0 {
		return nil
	}
	sync := needsSync(cacheDir)
	args := []string{"xbps-install", "--repository=" + repoDir}
	if sync {
		args = append(args, "-S")
	}
	args = append(args, "-y")
	args = append(args, pkgs...)
	if err := sysutil.RunInteractive("sudo", args...); err != nil {
		return fmt.Errorf("xbps-install failed for: %s", strings.Join(pkgs, " "))
	}
	// Only recorded on success: if the sync itself failed (network down,
	// etc.), the next call should retry rather than skip syncing for the
	// next repoSyncInterval on the strength of a sync that never happened.
	if sync {
		markSynced(cacheDir)
	}
	return nil
}

// Remove removes pkgs via sudo xbps-remove.
func Remove(pkgs ...string) error {
	args := append([]string{"xbps-remove", "-y"}, pkgs...)
	if err := sysutil.RunInteractive("sudo", args...); err != nil {
		return fmt.Errorf("xbps-remove failed")
	}
	return nil
}

// RepoPackage is one result from a repository search.
type RepoPackage struct {
	Name      string
	Version   string
	Desc      string
	Installed bool
}

// searchLineRe parses xbps-query -Rs output: "[*] name-1.2.3_1  description",
// where [*] means installed and [-] means available. The name/version split
// is left to xbps-uhelper rather than guessed, since pkgnames can contain
// dashes and digits.
var searchLineRe = regexp.MustCompile(`^\[([*-])\]\s+(\S+)\s*(.*)$`)

// SearchRepos searches Void's configured repositories (plus repoDir).
func SearchRepos(term, repoDir string) ([]RepoPackage, error) {
	out, err := sysutil.Output("xbps-query", "-Rs", term, "--repository="+repoDir)
	if err != nil {
		// xbps-query exits non-zero when nothing matched; that's not an error.
		return nil, nil
	}

	var pkgvers []string
	var raw [][3]string // marker, pkgver, desc
	for _, line := range strings.Split(out, "\n") {
		m := searchLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		raw = append(raw, [3]string{m[1], m[2], strings.TrimSpace(m[3])})
		pkgvers = append(pkgvers, m[2])
	}
	if len(raw) == 0 {
		return nil, nil
	}

	names, err := sysutil.Output("xbps-uhelper", append([]string{"getpkgname"}, pkgvers...)...)
	if err != nil {
		return nil, err
	}
	nameList := strings.Split(strings.TrimRight(names, "\n"), "\n")
	if len(nameList) != len(raw) {
		return nil, fmt.Errorf("search/getpkgname line count mismatch (%d vs %d)", len(raw), len(nameList))
	}

	pkgs := make([]RepoPackage, 0, len(raw))
	for i, r := range raw {
		version := strings.TrimPrefix(r[1], nameList[i]+"-")
		pkgs = append(pkgs, RepoPackage{
			Name:      nameList[i],
			Version:   version,
			Desc:      r[2],
			Installed: r[0] == "*",
		})
	}
	return pkgs, nil
}

// ShowRepo prints full metadata for a package from the repositories.
func ShowRepo(name, repoDir string) error {
	return sysutil.RunInteractive("xbps-query", "-R", "--repository="+repoDir, "-S", name)
}

// ExistsInRepos reports whether name resolves in Void's repos or repoDir.
func ExistsInRepos(name, repoDir string) bool {
	return IsAvailable(name, repoDir)
}

// ListInstalled returns every installed package name, in xbps's own order.
// Both the name and version come out of the same two commands here (one
// listing, one batched name-split) rather than querying per package --
// with ~1200 installed packages, per-package subprocesses turn an instant
// listing into a minutes-long one.
func ListInstalled() ([]InstalledPackage, error) {
	out, err := sysutil.Output("xbps-query", "-l")
	if err != nil {
		return nil, err
	}
	var pkgvers []string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pkgvers = append(pkgvers, fields[1])
	}
	if len(pkgvers) == 0 {
		return nil, nil
	}
	resolved, err := sysutil.Output("xbps-uhelper", append([]string{"getpkgname"}, pkgvers...)...)
	if err != nil {
		return nil, err
	}
	nameList := strings.Split(strings.TrimRight(resolved, "\n"), "\n")
	if len(nameList) != len(pkgvers) {
		return nil, fmt.Errorf("list/getpkgname line count mismatch (%d vs %d)", len(pkgvers), len(nameList))
	}

	pkgs := make([]InstalledPackage, 0, len(pkgvers))
	for i, pv := range pkgvers {
		pkgs = append(pkgs, InstalledPackage{
			Name:    nameList[i],
			Version: strings.TrimPrefix(pv, nameList[i]+"-"),
		})
	}
	return pkgs, nil
}

// InstalledPackage is one entry from ListInstalled.
type InstalledPackage struct {
	Name    string
	Version string
}

// Files lists the files a package owns, falling back to repository data
// when the package isn't installed locally (xbps-query -f is local-only,
// but -R answers the same question for anything in the repos).
func Files(name, repoDir string) error {
	if IsInstalled(name) {
		return sysutil.RunInteractive("xbps-query", "-f", name)
	}
	if !IsAvailable(name, repoDir) {
		return fmt.Errorf("package '%s' isn't installed and isn't in any configured repository", name)
	}
	return sysutil.RunInteractive("xbps-query", "-R", "--repository="+repoDir, "-f", name)
}

// Owns finds which package owns a file.
func Owns(path string) error { return sysutil.RunInteractive("xbps-query", "-o", path) }

// Deps lists a package's dependencies.
func Deps(name, repoDir string) error {
	return sysutil.RunInteractive("xbps-query", "-R", "--repository="+repoDir, "-x", name)
}

// RevDeps lists what depends on a package.
func RevDeps(name, repoDir string) error {
	return sysutil.RunInteractive("xbps-query", "-R", "--repository="+repoDir, "-X", name)
}

// Orphans lists packages installed only as dependencies and no longer needed.
func Orphans() error { return sysutil.RunInteractive("xbps-query", "-O") }

// Autoremove removes orphaned packages.
func Autoremove(noconfirm bool) error {
	args := []string{"xbps-remove", "-o"}
	if noconfirm {
		args = append(args, "-y")
	}
	return sysutil.RunInteractive("sudo", args...)
}

// Reconfigure re-runs a package's configuration step ("all" for everything).
func Reconfigure(name string) error {
	if name == "all" {
		return sysutil.RunInteractive("sudo", "xbps-reconfigure", "-a")
	}
	return sysutil.RunInteractive("sudo", "xbps-reconfigure", "-f", name)
}

// Repos lists the configured repositories.
func Repos() error { return sysutil.RunInteractive("xbps-query", "-L") }

// Hold marks packages as held back from updates; Unhold reverses it.
func Hold(names []string) error {
	args := append([]string{"xbps-pkgdb", "-m", "hold"}, names...)
	return sysutil.RunInteractive("sudo", args...)
}

func Unhold(names []string) error {
	args := append([]string{"xbps-pkgdb", "-m", "unhold"}, names...)
	return sysutil.RunInteractive("sudo", args...)
}

// ListHeld lists packages currently on hold.
func ListHeld() error { return sysutil.RunInteractive("xbps-query", "-H") }

// SyncRepos refreshes the repository indexes.
func SyncRepos(noconfirm bool) error {
	args := []string{"xbps-install", "-S"}
	if noconfirm {
		args = append(args, "-y")
	}
	return sysutil.RunInteractive("sudo", args...)
}

// ForceInstall reinstalls packages, overwriting existing files.
func ForceInstall(repoDir string, pkgs ...string) error {
	args := append([]string{"xbps-install", "--repository=" + repoDir, "-f", "-y"}, pkgs...)
	return sysutil.RunInteractive("sudo", args...)
}

// RemoveRecursive removes packages along with dependencies nothing else needs.
func RemoveRecursive(pkgs ...string) error {
	args := append([]string{"xbps-remove", "-R", "-y"}, pkgs...)
	return sysutil.RunInteractive("sudo", args...)
}

// SearchFile finds installed packages containing a matching file path.
func SearchFile(pattern string) error {
	return sysutil.RunInteractive("xbps-query", "-o", "*"+pattern+"*")
}

// ListAlternatives lists alternative candidate groups.
func ListAlternatives() error {
	return sysutil.RunInteractive("xbps-alternatives", "-l")
}

// SetAlternative selects the alternatives group provided by pkg.
func SetAlternative(pkg string) error {
	return sysutil.RunInteractive("sudo", "xbps-alternatives", "-s", pkg)
}

// CleanCache removes cached packages; deep also drops uninstalled ones.
func CleanCache(deep, noconfirm bool) error {
	flag := "-O"
	if deep {
		flag = "-OO"
	}
	args := []string{"xbps-remove", flag}
	if noconfirm {
		args = append(args, "-y")
	}
	return sysutil.RunInteractive("sudo", args...)
}

// AddRepo writes a new repository definition into /etc/xbps.d.
func AddRepo(url string) error {
	conf := fmt.Sprintf("repository=%s\n", url)
	tmp, err := os.CreateTemp("", "vpa-repo-*.conf")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(conf); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	name := strings.NewReplacer("/", "_", ":", "_", ".", "_").Replace(strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://"))
	dest := "/etc/xbps.d/10-vpa-" + name + ".conf"
	if err := sysutil.RunInteractive("sudo", "cp", tmp.Name(), dest); err != nil {
		return fmt.Errorf("couldn't write %s: %w", dest, err)
	}
	if err := sysutil.RunInteractive("sudo", "chmod", "0644", dest); err != nil {
		return err
	}
	return nil
}
