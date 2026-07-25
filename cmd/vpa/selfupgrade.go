package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"vpa/internal/sysutil"
	"vpa/internal/ui"
)

// vpaCheckout resolves the running binary back to the git checkout it was
// built from (through the ~/.local/bin symlink the installer creates), or
// an error explaining why self-update isn't possible for this install.
func vpaCheckout() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("couldn't find vpa's own binary path: %w", err)
	}
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("couldn't resolve vpa's binary path: %w", err)
	}
	root := filepath.Dir(real)

	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return "", fmt.Errorf("%s isn't a git checkout -- self-update only works when vpa was installed via install.sh or a manual git clone", root)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return "", fmt.Errorf("%s doesn't look like a vpa checkout (no go.mod)", root)
	}
	return root, nil
}

// selfUpdateIfAvailable checks vpa's own repo for a newer version and, if
// the checkout is behind, pulls and rebuilds. Reports either way, so
// `vpa update` visibly accounts for vpa itself rather than leaving you to
// wonder whether it was checked.
func selfUpdateIfAvailable() error {
	ui.Info("checking for a newer vpa...")
	root, err := vpaCheckout()
	if err != nil {
		return err
	}

	if err := sysutil.RunQuiet("git", "-C", root, "fetch", "--quiet"); err != nil {
		return fmt.Errorf("git fetch failed: %w", err)
	}

	local, err1 := sysutil.Output("git", "-C", root, "rev-parse", "HEAD")
	remote, err2 := sysutil.Output("git", "-C", root, "rev-parse", "@{u}")
	if err1 != nil || err2 != nil {
		return fmt.Errorf("couldn't compare local and upstream revisions")
	}
	if strings.TrimSpace(local) == strings.TrimSpace(remote) {
		ui.Ok("vpa is already up to date")
		return nil
	}

	ui.Info("a newer vpa is available -- updating %s", root)
	if !ui.Confirm("Update vpa itself now?") {
		return nil
	}
	return pullAndRebuild(root)
}

func pullAndRebuild(root string) error {
	if err := sysutil.RunInteractive("git", "-C", root, "pull", "--quiet"); err != nil {
		return fmt.Errorf("git pull failed: %w", err)
	}

	ui.Info("rebuilding vpa")
	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", "vpa", "./cmd/vpa")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build failed: %w", err)
	}

	ui.Ok("vpa updated -- the new version applies from your next vpa command")
	return nil
}
