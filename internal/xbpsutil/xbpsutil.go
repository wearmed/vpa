// Package xbpsutil wraps the xbps-* CLI tools vur needs.
package xbpsutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"vur/internal/sysutil"
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

// Create builds pkgname-pkgver_pkgrel.<arch>.xbps from pkgdir into repoDir.
func Create(pkgname, pkgver, pkgrel, pkgdir, deps, desc, url, license, repoDir string) error {
	sysutil.RequireBin("xbps-create", "xbps")

	if pkgname == "" {
		return fmt.Errorf("create_xbps_pkg: empty pkgname (broken PKGBUILD?)")
	}
	if pkgver == "" {
		return fmt.Errorf("%s: empty pkgver -- likely a -git/-svn/-hg PKGBUILD whose pkgver() vur doesn't invoke; set pkgver= manually with --edit", pkgname)
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
	args := []string{"-A", arch, "-n", fmt.Sprintf("%s-%s_%s", pkgname, pkgver, pkgrel), "-s", desc}
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

// Install installs pkgs from repoDir via sudo xbps-install.
func Install(repoDir string, pkgs ...string) error {
	if len(pkgs) == 0 {
		return nil
	}
	args := append([]string{"xbps-install", "--repository=" + repoDir, "-Sy"}, pkgs...)
	if err := sysutil.RunInteractive("sudo", args...); err != nil {
		return fmt.Errorf("xbps-install failed for: %s", strings.Join(pkgs, " "))
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
