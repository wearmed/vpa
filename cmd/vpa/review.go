package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"vpa/internal/pkgbuild"
	"vpa/internal/sysutil"
	"vpa/internal/systemdcheck"
	"vpa/internal/ui"
)

// reviewAndLoad is the single enforcement point for "show + confirm before
// ever sourcing untrusted PKGBUILD content".
//
// AUR packages are build scripts written by strangers, and building one
// runs their code on your machine -- that's inherent to the AUR, not
// something vpa can design away. What vpa can do is make the decision
// understandable: lead with a plain summary (what this is, where the code
// comes from, what it pulls in) instead of a wall of bash most people
// won't actually read, keep the full script one keypress away, and say
// plainly what approving it means.
func (a *App) reviewAndLoad(pkgbase, dir string) (*pkgbuild.PKGBUILD, error) {
	snapshot := filepath.Join(a.Cfg.ReviewedDir, pkgbase+".PKGBUILD")
	pkgbuildPath := filepath.Join(dir, "PKGBUILD")

	cur, err := os.ReadFile(pkgbuildPath)
	if err != nil {
		return nil, err
	}
	prev, prevErr := os.ReadFile(snapshot)
	seenBefore := prevErr == nil
	unchanged := seenBefore && bytes.Equal(prev, cur)

	installScripts, _ := filepath.Glob(filepath.Join(dir, "*.install"))
	printSummary(pkgbase, pkgbuild.Summarize(cur), len(installScripts) > 0)

	if unchanged {
		ui.Ok("You approved this exact build script before, and it hasn't changed since.")
	} else if seenBefore {
		ui.Warn("This build script has CHANGED since you last approved it -- press 'd' to see what changed.")
	}

	systemdcheck.Warn(pkgbuildPath)

	for {
		choices := "ynv"
		prompt := "[Y]es / [n]o / [v]iew script"
		if seenBefore && !unchanged {
			prompt += " / [d]iff"
			choices += "d"
		}
		if a.Cfg.EditPKGBUILD {
			prompt += " / [e]dit"
			choices += "e"
		}

		switch ui.Ask(prompt+"?", choices, "Install '%s'?", pkgbase) {
		case "y":
			if err := os.WriteFile(snapshot, cur, 0o644); err != nil {
				return nil, err
			}
			return pkgbuild.Load(dir)

		case "n":
			return nil, fmt.Errorf("cancelled -- '%s' was not installed", pkgbase)

		case "v":
			fmt.Println()
			os.Stdout.Write(cur)
			for _, m := range installScripts {
				fmt.Printf("\n--- %s (runs when the package is installed) ---\n", filepath.Base(m))
				data, _ := os.ReadFile(m)
				os.Stdout.Write(data)
			}
			fmt.Println()

		case "d":
			fmt.Println()
			printDiff(snapshot, pkgbuildPath)

		case "e":
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
			if edited, err := os.ReadFile(pkgbuildPath); err == nil {
				cur = edited
				printSummary(pkgbase, pkgbuild.Summarize(cur), len(installScripts) > 0)
			}
		}
	}
}

// printSummary shows the plain-language overview a user actually needs in
// order to decide, rather than making them parse bash to find it.
func printSummary(pkgbase string, s pkgbuild.Summary, hasInstallScript bool) {
	name := s.Name
	if name == "" {
		name = pkgbase
	}

	fmt.Println()
	fmt.Printf("  %s %s\n", ui.Bold(name), s.Version)
	if s.Description != "" {
		fmt.Printf("  %s\n", s.Description)
	}
	fmt.Println()
	if s.URL != "" {
		fmt.Printf("  Project      %s\n", s.URL)
	}
	if hosts := s.SourceHosts(); len(hosts) > 0 {
		fmt.Printf("  Downloads    %s\n", strings.Join(hosts, ", "))
	}
	if len(s.Depends) > 0 {
		fmt.Printf("  Needs        %s\n", strings.Join(s.Depends, ", "))
	}
	if s.License != "" {
		fmt.Printf("  License      %s\n", s.License)
	}
	fmt.Println()

	ui.Warn("This is a build script from the AUR -- written by a stranger, not checked by Void or by vpa. Installing it runs their code on your machine. Press 'v' to read it first if you're unsure.")
	if hasInstallScript || s.HasInstall {
		ui.Warn("It also ships an install script, which runs automatically when the package is installed.")
	}
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
			ui.Warn("couldn't show what changed: %v", err)
			return
		}
	}
	fmt.Println(out)
}
