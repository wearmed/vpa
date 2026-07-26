package main

import "fmt"

// canonicalCommand resolves any recognized command name or alias to its
// canonical form, or "" if unrecognized. The single source of truth for
// aliases, used by both dispatch and help lookup so they can never drift
// apart.
func canonicalCommand(name string) string {
	switch name {
	case "search", "s":
		return "search"
	case "info":
		return "info"
	case "install", "i":
		return "install"
	case "devinstall", "di":
		return "devinstall"
	case "forceinstall", "fi":
		return "forceinstall"
	case "downgrade", "dg", "rollback":
		return "downgrade"
	case "remove", "rm":
		return "remove"
	case "removerecursive", "rr":
		return "removerecursive"
	case "update", "up", "upgrade":
		return "update"
	case "sync", "sy":
		return "sync"
	case "list", "ls":
		return "list"
	case "filelist", "fl", "files":
		return "filelist"
	case "whatprovides", "wp", "owns":
		return "whatprovides"
	case "searchfile", "sf":
		return "searchfile"
	case "deps":
		return "deps"
	case "reverse", "rv", "revdeps":
		return "reverse"
	case "orphans":
		return "orphans"
	case "autoremove", "ar":
		return "autoremove"
	case "reconfigure", "rc":
		return "reconfigure"
	case "listrepos", "lr", "repolist", "rl", "repos":
		return "listrepos"
	case "addrepo":
		return "addrepo"
	case "listalternatives", "la":
		return "listalternatives"
	case "setalternative", "sa":
		return "setalternative"
	case "cleanup", "cl", "clean":
		return "cleanup"
	case "hold":
		return "hold"
	case "unhold":
		return "unhold"
	case "help", "h", "?":
		return "help"
	case "helppager", "hp":
		return "helppager"
	case "version":
		return "version"
	default:
		return ""
	}
}

var commandHelp = map[string]string{
	"search": `vpa search (s) <term>
vpa search cat <category>

Searches Void's repos, the AUR and Flathub at once.
Void results come first. A native package is usually the better choice.
Anything you already have is marked [installed].
A neglected AUR package is marked [orphaned], [out of date] or
[unmaintained for ...].

Category search finds things by what they are, not by name.
"cat" with nothing after it lists every category.

  vpa search firefox
  vpa search cat browser
  vpa search cat

Categories are curated. Add your own in ~/.config/vpa/categories.conf.
`,
	"info": `vpa info <pkg>

Shows details for a package.
Looks in Void's repos, the AUR, or Flathub if you give an app ID.

  vpa info firefox
  vpa info org.mozilla.firefox
`,
	"install": `vpa install (i) <pkg> [pkg...]

Installs packages. vpa works out where each one comes from:

  in Void's repos           installed directly, nothing is built
  in the AUR                shows you what it is, then builds it
  a search term             opens a numbered list to pick from
  a .xbps file or URL       installed directly
  a .deb / .rpm / Arch file unpacked and repackaged for Void
  a Flatpak app ID          installed from Flathub

Native packages win. A Flatpak needs its full app ID, or --flatpak.
With --flatpak a short name works. vpa asks if it is ambiguous.
An orphaned or long-unmaintained AUR package asks a second time.

  vpa install firefox
  vpa install pipes.sh
  vpa install ./something.deb
  vpa install org.mozilla.firefox
  vpa install --flatpak gimp

Flags: --edit, --flatpak, --noconfirm
`,
	"devinstall": `vpa devinstall (di) <pkg> [pkg...]

Installs packages plus their -devel counterparts.
Those hold the headers you need to build software against them.
Packages with no -devel counterpart are installed on their own.

  vpa devinstall openssl
`,
	"forceinstall": `vpa forceinstall (fi) <pkg> [pkg...]

Reinstalls packages, overwriting files already on disk.
Use this to repair a package whose files got damaged.
Use "vpa install" for everything else.

  vpa forceinstall firefox
`,
	"downgrade": `vpa downgrade (dg, rollback) <pkg> [version]

Rolls a package back to an earlier version.

Only versions still in /var/cache/xbps can be used, which means ones you
have installed before. A repository only ever offers the current version.
Naming a version skips the picker.

  vpa downgrade firefox
  vpa downgrade firefox 152.0_1

"vpa cleanup" empties that cache, so it also throws away what you could
roll back to. After a downgrade, "vpa update" upgrades the package again
unless you "vpa hold" it.
`,
	"remove": `vpa remove (rm) <pkg> [pkg...]

Removes packages. Works on installed Flatpaks too, given their app ID.

  vpa remove firefox
  vpa remove org.mozilla.firefox
`,
	"removerecursive": `vpa removerecursive (rr) <pkg> [pkg...]

Removes packages, plus dependencies nothing else needs anymore.

  vpa removerecursive firefox
`,
	"update": `vpa update (up, upgrade)

Updates everything, in this order:

  1. vpa itself, if there is a newer version
  2. your Flatpak applications
  3. all your installed packages
  4. anything vpa built from the AUR

  vpa update

Flags: --devel (also rebuild -git packages when upstream moved)
`,
	"sync": `vpa sync (sy)

Refreshes the list of available packages. Installs nothing.
"vpa update" does this for you, so you rarely need it.

  vpa sync
`,
	"list": `vpa list (ls) [--aur]

Lists every package installed on your system.
AUR packages are tagged (aur), Flatpaks (flatpak).

  vpa list
  vpa list --aur     only the AUR ones
`,
	"filelist": `vpa filelist (fl) <pkg>

Lists the files a package installs.
Works on packages you do not have installed.

  vpa filelist firefox
`,
	"whatprovides": `vpa whatprovides (wp) <file>

Shows which package a file came from.

  vpa whatprovides /usr/bin/firefox
`,
	"searchfile": `vpa searchfile (sf) <file>

Finds installed packages containing a matching file.
Use it when you know part of a filename but not the package.

  vpa searchfile libssl
`,
	"deps": `vpa deps <pkg>

Shows what a package needs in order to work.

  vpa deps firefox
`,
	"reverse": `vpa reverse (rv) <pkg>

Shows everything that depends on a package.
That is what would break if you removed it.

  vpa reverse openssl
`,
	"orphans": `vpa orphans

Lists packages installed only as dependencies and no longer needed.
"vpa autoremove" clears them out.

  vpa orphans
`,
	"autoremove": `vpa autoremove (ar)

Removes packages installed only as dependencies and no longer needed.
See "vpa orphans" to check the list first.

  vpa autoremove
`,
	"reconfigure": `vpa reconfigure (rc) <pkg|all>

Re-runs a package's setup step.
Use it if a package did not finish configuring properly.

  vpa reconfigure firefox
  vpa reconfigure all
`,
	"listrepos": `vpa listrepos (lr)

Shows the repositories vpa installs from.

  vpa listrepos
`,
	"addrepo": `vpa addrepo <url>

Adds another repository to install from.
Only add repositories you trust. Anything in them can install files
anywhere on your system.

  vpa addrepo https://repo-default.voidlinux.org/current/nonfree
`,
	"listalternatives": `vpa listalternatives (la)

Shows which package provides things several packages can provide.
For example a default text editor or C compiler.

  vpa listalternatives
`,
	"setalternative": `vpa setalternative (sa) <pkg>

Chooses which package provides an alternative.
See "vpa listalternatives" for what is configurable.

  vpa setalternative vim
`,
	"cleanup": `vpa cleanup (cl)

Frees up disk space.
Clears vpa's build files and downloaded packages you no longer need.
Safe to run whenever.

  vpa cleanup
`,
	"hold": `vpa hold [pkg...]

Stops packages being updated by "vpa update".
With no arguments, shows what is currently held.

  vpa hold firefox
  vpa hold
`,
	"unhold": `vpa unhold <pkg> [pkg...]

Allows held packages to be updated again.

  vpa unhold firefox
`,
	"version": `vpa version

Shows which version of vpa this is. Same as "vpa --version".

  vpa version
`,
}

// printCommandHelp prints canonical name's specific help text if known,
// returning whether it found one.
func printCommandHelp(name string) bool {
	h, ok := commandHelp[canonicalCommand(name)]
	if ok {
		fmt.Print(h)
	}
	return ok
}
