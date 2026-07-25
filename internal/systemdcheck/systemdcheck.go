// Package systemdcheck statically flags PKGBUILD build flags that
// unconditionally require systemd, which doesn't exist on Void (runit) --
// the same problem Artix's OpenRC/runit/s6 users hit building ordinary AUR
// packages on top of Arch.
package systemdcheck

import (
	"os"
	"regexp"

	"vur/internal/ui"
)

var forcingFlags = regexp.MustCompile(
	`--(with|enable)-systemd|-Dsystemd=(true|enabled)|-D(WITH_)?SYSTEMD(_SUPPORT)?=(ON|1)|USE_SYSTEMD=1`,
)

// Warn scans a PKGBUILD file for build flags that unconditionally require
// systemd and prints an actionable warning if found. Never edits the file;
// --edit is how the user acts on it.
func Warn(pkgbuildPath string) {
	data, err := os.ReadFile(pkgbuildPath)
	if err != nil {
		return
	}
	hits := forcingFlags.FindAllString(string(data), -1)
	if len(hits) == 0 {
		return
	}
	ui.Warn("this PKGBUILD unconditionally requests systemd support, which doesn't exist on Void (runit):")
	seen := make(map[string]bool)
	for _, h := range hits {
		if seen[h] {
			continue
		}
		seen[h] = true
		os.Stderr.WriteString("    " + h + "\n")
	}
	ui.Warn("the build will likely fail unless you --edit it first and flip these to their disabled form (--without-systemd, --disable-systemd, -Dsystemd=false/disabled, -D(WITH_)SYSTEMD=OFF, etc.)")
}
