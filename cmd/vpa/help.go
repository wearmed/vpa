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

Search Void's repositories, the AUR and Flathub at the same time. Void
results come first -- a native package is nearly always the better
choice when one exists. Anything you already have is marked
[installed].

  vpa search firefox

Searching a category shows you what's out there when you know the kind
of thing you want but not its name. 'cat' with no category after it
lists every category there is.

  vpa search cat browser
  vpa search cat games
  vpa search cat

AUR results carry a warning when a package looks neglected -- orphaned,
flagged out of date, or untouched for a long time.

Categories are curated rather than detected: nothing in xbps or the AUR
records what kind of thing a package is. vpa ships a list, refreshes it
from the server weekly, and you can add your own in
~/.config/vpa/categories.conf.
`,
	"info": `vpa info <pkg>

Show details for a package, from wherever it exists: Void's
repositories, the AUR, both, or Flathub if you give an app ID.

  vpa info firefox
  vpa info org.mozilla.firefox
`,
	"install": `vpa install (i) <pkg> [pkg...]

Install packages. vpa works out where each one comes from:

  from Void's repositories   installed directly, nothing is built
  from the AUR               shows you what it is, then builds it
  a search term              opens a numbered list to pick from
  a .xbps file or URL        installed directly
  a .deb / .rpm / Arch file  unpacked and repackaged for Void
  a Flatpak app ID           installed from Flathub

Native packages are preferred: a Flatpak is only installed if you give
its full app ID, or pass --flatpak.

  vpa install firefox
  vpa install pipes.sh
  vpa install ./something.deb
  vpa install org.mozilla.firefox
  vpa install --flatpak gimp

With --flatpak you can use a short name; vpa looks it up on Flathub and
asks which one you meant if it's ambiguous.

Options: --edit (edit an AUR build script first), --flatpak, --noconfirm
`,
	"devinstall": `vpa devinstall (di) <pkg> [pkg...]

Install packages along with their -devel counterparts (the headers and
files you need to build software against them). Packages with no
-devel counterpart are installed on their own.

  vpa devinstall openssl
`,
	"forceinstall": `vpa forceinstall (fi) <pkg> [pkg...]

Reinstall packages, overwriting files already on disk. This is for
repairing a package whose files got damaged -- use 'vpa install' for
everything else.
`,
	"remove": `vpa remove (rm) <pkg> [pkg...]

Remove packages, including installed Flatpaks (given their app ID).

  vpa remove firefox
  vpa remove org.mozilla.firefox
`,
	"removerecursive": `vpa removerecursive (rr) <pkg> [pkg...]

Remove packages, plus any dependencies they pulled in that nothing
else needs anymore.
`,
	"update": `vpa update (up, upgrade)

Update everything, in this order:

  1. vpa itself, if there's a newer version
  2. your Flatpak applications
  3. all your installed packages
  4. anything vpa built from the AUR

  vpa update

Options: --devel (also rebuild -git packages when their upstream code
changed, even if the version number didn't)
`,
	"sync": `vpa sync (sy)

Refresh the list of available packages from your repositories, without
installing or updating anything. 'vpa update' does this for you, so you
rarely need to run it yourself.
`,
	"list": `vpa list (ls) [--aur]

List every package installed on your system. Packages vpa built from
the AUR are tagged (aur), and Flatpak applications (flatpak). Pass
--aur to list only the AUR ones.
`,
	"filelist": `vpa filelist (fl) <pkg>

List the files a package installs. Works for packages you don't have
installed too.

  vpa filelist firefox
`,
	"whatprovides": `vpa whatprovides (wp) <file>

Show which package a file came from.

  vpa whatprovides /usr/bin/firefox
`,
	"searchfile": `vpa searchfile (sf) <file>

Find installed packages containing a file matching what you type. Use
this when you know part of a filename but not the package.

  vpa searchfile libssl
`,
	"deps": `vpa deps <pkg>

Show what a package needs in order to work.
`,
	"reverse": `vpa reverse (rv) <pkg>

Show what would break if you removed a package -- everything that
depends on it.
`,
	"orphans": `vpa orphans

List packages that were only installed as dependencies and aren't
needed by anything anymore. 'vpa autoremove' clears them out.
`,
	"autoremove": `vpa autoremove (ar)

Remove packages that were only installed as dependencies and aren't
needed anymore (see 'vpa orphans').
`,
	"reconfigure": `vpa reconfigure (rc) <pkg|all>

Re-run a package's setup step. Useful if a package didn't finish
configuring properly. Pass 'all' to do every installed package.
`,
	"listrepos": `vpa listrepos (lr)

Show the repositories vpa installs from.
`,
	"addrepo": `vpa addrepo <url>

Add another repository to install packages from. Only add repositories
you trust: anything in them can install files anywhere on your system.

  vpa addrepo https://repo-default.voidlinux.org/current/nonfree
`,
	"listalternatives": `vpa listalternatives (la)

Show which package currently provides things that several packages can
provide (like a default text editor or C compiler).
`,
	"setalternative": `vpa setalternative (sa) <pkg>

Choose which package provides an alternative (see
'vpa listalternatives').
`,
	"cleanup": `vpa cleanup (cl)

Free up disk space: clears vpa's build files and downloaded packages
you no longer need. Safe to run whenever.
`,
	"hold": `vpa hold [pkg...]

Stop packages being updated by 'vpa update'. With no arguments, shows
what's currently held.
`,
	"version": `vpa version

Show which version of vpa this is. Same as 'vpa --version'.
`,
	"unhold": `vpa unhold <pkg> [pkg...]

Allow held packages to be updated again.
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
