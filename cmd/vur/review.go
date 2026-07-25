package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"vur/internal/pkgbuild"
	"vur/internal/sysutil"
	"vur/internal/systemdcheck"
	"vur/internal/ui"
)

// reviewAndLoad is the single enforcement point for "show + confirm before
// ever sourcing untrusted PKGBUILD content". Diffs against the last
// reviewed snapshot instead of re-dumping an unchanged PKGBUILD every time,
// warns about systemd-forcing build flags, honors --edit, then confirms.
func (a *App) reviewAndLoad(pkgbase, dir string) (*pkgbuild.PKGBUILD, error) {
	snapshot := filepath.Join(a.Cfg.ReviewedDir, pkgbase+".PKGBUILD")
	pkgbuildPath := filepath.Join(dir, "PKGBUILD")

	cur, err := os.ReadFile(pkgbuildPath)
	if err != nil {
		return nil, err
	}
	prev, prevErr := os.ReadFile(snapshot)

	switch {
	case prevErr == nil && bytes.Equal(prev, cur):
		ui.Info("PKGBUILD for %s is unchanged since last review", pkgbase)
	case prevErr == nil:
		ui.Info("PKGBUILD for %s changed since last review:", pkgbase)
		printDiff(snapshot, pkgbuildPath)
	default:
		ui.Info("PKGBUILD for %s:", pkgbase)
		os.Stdout.Write(cur)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "*.install"))
	for _, m := range matches {
		ui.Info("install scriptlet: %s", filepath.Base(m))
		data, _ := os.ReadFile(m)
		os.Stdout.Write(data)
	}

	systemdcheck.Warn(pkgbuildPath)

	if a.Cfg.EditPKGBUILD {
		if ui.Confirm("Open PKGBUILD for %s in %s before building?", pkgbase, a.Cfg.Editor) {
			// $EDITOR/$VISUAL commonly carry arguments (e.g. "code --wait",
			// "vim -u NONE") -- treating the whole string as one literal
			// binary name would fail to even start.
			fields := strings.Fields(a.Cfg.Editor)
			if len(fields) == 0 {
				fields = []string{"vi"}
			}
			args := append(append([]string{}, fields[1:]...), pkgbuildPath)
			if err := sysutil.RunInteractive(fields[0], args...); err != nil {
				ui.Warn("editor exited with an error: %v", err)
			}
		}
	}

	if !ui.Confirm("Build and install '%s' using the PKGBUILD shown above?", pkgbase) {
		return nil, fmt.Errorf("aborted by user for %s", pkgbase)
	}

	final, err := os.ReadFile(pkgbuildPath)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(snapshot, final, 0o644); err != nil {
		return nil, err
	}

	return pkgbuild.Load(dir)
}

func printDiff(oldFile, newFile string) {
	sysutil.RequireBin("diff", "diffutils")
	out, err := sysutil.Output("diff", "-u", "--", oldFile, newFile)
	// diff exits 1 to mean "files differ", which is the expected case here,
	// not a real failure -- anything else (missing binary, permissions,
	// exit 2) is worth telling the user about instead of silently printing
	// nothing and letting them think there were no changes.
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
			ui.Warn("diff failed, showing no changes (this doesn't mean there weren't any): %v", err)
			return
		}
	}
	fmt.Println(out)
}
