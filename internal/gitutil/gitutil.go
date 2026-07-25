// Package gitutil wraps the small set of git operations vpa needs.
package gitutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"vpa/internal/config"
	"vpa/internal/ui"
)

// CloneAUR clones (or updates) an AUR package base's git repo into dest.
// Falls back to a fresh clone if updating an existing checkout fails --
// covers both transient network blips and a corrupted .git left behind by
// an interrupted previous run.
func CloneAUR(pkgbase, dest string) error {
	if isGitDir(dest) {
		ui.Info("updating existing checkout of %s", pkgbase)
		fetchErr := run(dest, "fetch", "--quiet", "origin")
		var resetErr error
		if fetchErr == nil {
			resetErr = run(dest, "reset", "--hard", "--quiet", "origin/HEAD")
		}
		if fetchErr != nil || resetErr != nil {
			ui.Warn("existing checkout of %s looks broken -- re-cloning fresh", pkgbase)
			os.RemoveAll(dest)
		}
	}
	if !isGitDir(dest) {
		os.RemoveAll(dest)
		cmd := exec.Command("git", "clone", "--quiet", config.AURGit+"/"+pkgbase+".git", dest)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git clone failed for %s -- check the name and your network connection", pkgbase)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "PKGBUILD")); err != nil {
		return fmt.Errorf("%s: cloned repo has no PKGBUILD -- is '%s' really an AUR package base?", pkgbase, pkgbase)
	}
	return nil
}

func isGitDir(dest string) bool {
	fi, err := os.Stat(filepath.Join(dest, ".git"))
	return err == nil && fi.IsDir()
}

func run(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// LsRemoteHead returns the HEAD commit of a remote repo via a cheap `git
// ls-remote` (no clone needed). Empty string if it can't be determined.
func LsRemoteHead(remoteURL string) string {
	out, err := exec.Command("git", "ls-remote", remoteURL, "HEAD").Output()
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// HeadCommit returns the current HEAD commit of a local checkout, or "" if
// dir isn't a git repo.
func HeadCommit(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// CloneWorkingCopy does a plain (non-bare) clone of url into dest, used for
// git+ PKGBUILD sources. Optionally checks out a specific ref.
func CloneWorkingCopy(url, dest, ref string) error {
	os.RemoveAll(dest)
	cmd := exec.Command("git", "clone", "--quiet", url, dest)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed for %s (%s)", dest, url)
	}
	if ref != "" {
		if err := run(dest, "checkout", "--quiet", ref); err != nil {
			return fmt.Errorf("git checkout '%s' failed for %s", ref, dest)
		}
	}
	return nil
}
