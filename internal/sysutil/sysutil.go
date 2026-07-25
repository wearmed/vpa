// Package sysutil provides small OS-level helpers shared across vur.
package sysutil

import (
	"os"
	"os/exec"

	"vur/internal/ui"
)

// RequireBin aborts unless bin is on PATH, offering to install pkg via xbps first.
func RequireBin(bin, pkg string) {
	if _, err := exec.LookPath(bin); err == nil {
		return
	}
	ui.Warn("'%s' is required but not installed (package: %s).", bin, pkg)
	if !ui.Confirm("Install %s now with sudo xbps-install?", pkg) {
		ui.Die("cannot continue without %s", bin)
	}
	cmd := exec.Command("sudo", "xbps-install", "-Sy", pkg)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	if err := cmd.Run(); err != nil {
		ui.Die("failed to install %s: %v", pkg, err)
	}
}

// RunInteractive runs a command with stdio wired to the terminal.
func RunInteractive(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	return cmd.Run()
}

// RunQuiet runs a command, discarding stdout but keeping stderr for error diagnostics.
func RunQuiet(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunSilent runs a command, discarding both stdout and stderr; used for probes
// where a non-zero exit is an expected, non-error outcome (existence checks).
func RunSilent(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}

// Output runs a command and returns trimmed stdout.
func Output(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return string(out), err
}
