// Package flatpak wraps the flatpak CLI.
//
// Flatpak is the one thing vpa fronts that it can't convert into a real
// .xbps package: flatpaks are sandboxed bundles with their own runtimes
// and their own database, so vpa drives flatpak directly rather than
// repackaging. That means a flatpak is tracked by flatpak, not by xbps --
// worth knowing, since everything else vpa installs ends up in xbps.
package flatpak

import (
	"fmt"
	"strings"

	"vpa/internal/sysutil"
)

// App is one Flatpak application.
type App struct {
	ID      string // reverse-DNS application ID, e.g. org.mozilla.firefox
	Name    string
	Version string
	Desc    string
	Origin  string
}

// Available reports whether the flatpak command exists at all. Everything
// else in this package should be guarded by it, since Flatpak is optional
// on Void rather than assumed.
func Available() bool {
	return sysutil.Has("flatpak")
}

// LooksLikeAppID reports whether a name is a reverse-DNS Flatpak app ID
// (org.mozilla.firefox). Used to route an install without a flag: nothing
// in Void's repos or the AUR is named like this, so it's unambiguous.
func LooksLikeAppID(name string) bool {
	if strings.Count(name, ".") < 2 {
		return false
	}
	for _, part := range strings.Split(name, ".") {
		if part == "" {
			return false
		}
	}
	// A filename that happens to have dots isn't an app ID.
	for _, suffix := range []string{".xbps", ".deb", ".rpm", ".zst", ".xz", ".gz", ".sh", ".git"} {
		if strings.HasSuffix(strings.ToLower(name), suffix) {
			return false
		}
	}
	return !strings.ContainsAny(name, "/\\ ")
}

// Search queries the configured remotes. Flatpak exits non-zero when
// nothing matches, which isn't an error worth surfacing.
func Search(term string) ([]App, error) {
	out, err := sysutil.Output("flatpak", "search", "--columns=application,version,description", term)
	if err != nil {
		return nil, nil
	}
	var apps []App
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 1 || fields[0] == "" {
			continue
		}
		a := App{ID: fields[0]}
		if len(fields) > 1 {
			a.Version = fields[1]
		}
		if len(fields) > 2 {
			a.Desc = fields[2]
		}
		apps = append(apps, a)
	}
	return apps, nil
}

// List returns installed applications. Runtimes and extensions are
// excluded: they're pulled in automatically as dependencies, and listing
// them alongside real applications is just noise.
func List() ([]App, error) {
	out, err := sysutil.Output("flatpak", "list", "--app", "--columns=application,version,origin")
	if err != nil {
		return nil, err
	}
	var apps []App
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		a := App{ID: fields[0]}
		if len(fields) > 1 {
			a.Version = fields[1]
		}
		if len(fields) > 2 {
			a.Origin = fields[2]
		}
		apps = append(apps, a)
	}
	return apps, nil
}

// IsInstalled reports whether an app ID is installed.
func IsInstalled(id string) bool {
	return sysutil.RunSilent("flatpak", "info", id) == nil
}

// Info prints details for an application. `flatpak info` only knows about
// installed apps, so fall back to querying the remotes for one that isn't
// installed yet -- otherwise looking up something before installing it,
// which is the common case, just errors.
func Info(id string) error {
	if IsInstalled(id) {
		return sysutil.RunInteractive("flatpak", "info", id)
	}
	remotes, err := sysutil.Output("flatpak", "remotes", "--columns=name")
	if err == nil {
		for _, r := range strings.Fields(remotes) {
			if sysutil.RunSilent("flatpak", "remote-info", r, id) == nil {
				return sysutil.RunInteractive("flatpak", "remote-info", r, id)
			}
		}
	}
	return fmt.Errorf("no Flatpak called %q is installed or available from your remotes", id)
}

// Install installs applications from the configured remotes.
func Install(noconfirm bool, ids ...string) error {
	args := []string{"install"}
	if noconfirm {
		args = append(args, "-y")
	}
	args = append(args, ids...)
	return sysutil.RunInteractive("flatpak", args...)
}

// Remove uninstalls applications.
func Remove(noconfirm bool, ids ...string) error {
	args := []string{"uninstall"}
	if noconfirm {
		args = append(args, "-y")
	}
	args = append(args, ids...)
	return sysutil.RunInteractive("flatpak", args...)
}

// Update updates all installed applications.
func Update(noconfirm bool) error {
	args := []string{"update"}
	if noconfirm {
		args = append(args, "-y")
	}
	return sysutil.RunInteractive("flatpak", args...)
}

// RemoveUnused drops runtimes nothing installed still needs -- Flatpak's
// equivalent of orphaned packages, and usually where the disk space is.
func RemoveUnused(noconfirm bool) error {
	args := []string{"uninstall", "--unused"}
	if noconfirm {
		args = append(args, "-y")
	}
	return sysutil.RunInteractive("flatpak", args...)
}
