package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"vur/internal/sysutil"
	"vur/internal/ui"
)

// cmdSelfUpgrade updates vur itself: git pull in whatever checkout the
// running binary was built from, then rebuild in place. Mirrors exactly
// what install.sh does, so it works the same whether that's the default
// ~/.local/share/vur clone from the curl installer or a manual `git clone`.
func cmdSelfUpgrade() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("couldn't find vur's own binary path: %w", err)
	}
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("couldn't resolve vur's binary path: %w", err)
	}
	root := filepath.Dir(real)

	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return fmt.Errorf("%s doesn't look like a git checkout (no .git found) -- self-update only works when vur was installed via install.sh or a manual git clone; re-run the curl installer instead", root)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return fmt.Errorf("%s doesn't look like a vur checkout (no go.mod found)", root)
	}

	ui.Info("updating vur checkout at %s", root)
	if err := sysutil.RunInteractive("git", "-C", root, "pull"); err != nil {
		return fmt.Errorf("git pull failed: %w", err)
	}

	ui.Info("rebuilding vur")
	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", "vur", "./cmd/vur")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build failed: %w", err)
	}

	ui.Ok("vur updated -- this running process is still the old version, re-run to use the new one")
	return nil
}
