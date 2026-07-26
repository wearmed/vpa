package main

import (
	"strings"
	"testing"
)

// Every alias has to resolve to its canonical command. canonicalCommand is
// the single source of truth for both dispatch and help lookup, so an alias
// missing here is an alias that silently doesn't exist.
func TestRemoveAliases(t *testing.T) {
	for _, a := range []string{"remove", "rm", "uninstall", "delete", "purge"} {
		if got := canonicalCommand(a); got != "remove" {
			t.Errorf("canonicalCommand(%q) = %q, want remove", a, got)
		}
	}
}

// A command's help text should name the aliases it answers to, or they are
// undiscoverable.
func TestRemoveHelpListsAliases(t *testing.T) {
	h := commandHelp["remove"]
	for _, a := range []string{"rm", "uninstall", "delete", "purge"} {
		if !strings.Contains(h, a) {
			t.Errorf("remove help does not mention alias %q", a)
		}
	}
}

// Two commands claiming the same alias would make dispatch depend on the
// order of the switch, which is the kind of thing nobody notices until it
// bites.
func TestNoDuplicateAliases(t *testing.T) {
	seen := map[string]string{}
	for _, a := range []string{
		"search", "s", "info", "install", "i", "devinstall", "di",
		"forceinstall", "fi", "downgrade", "dg", "rollback",
		"remove", "rm", "uninstall", "delete", "purge",
		"removerecursive", "rr", "update", "up", "upgrade", "sync", "sy",
		"list", "ls", "filelist", "fl", "files", "whatprovides", "wp", "owns",
		"searchfile", "sf", "deps", "reverse", "rv", "revdeps", "orphans",
		"autoremove", "ar", "reconfigure", "rc", "listrepos", "lr",
		"repolist", "rl", "repos", "addrepo", "listalternatives", "la",
		"setalternative", "sa", "cleanup", "cl", "clean", "hold", "unhold",
		"help", "h", "?", "helppager", "hp", "version",
	} {
		canon := canonicalCommand(a)
		if canon == "" {
			t.Errorf("alias %q resolves to nothing", a)
			continue
		}
		if prev, dup := seen[a]; dup {
			t.Errorf("alias %q claimed by both %q and %q", a, prev, canon)
		}
		seen[a] = canon
	}
}
