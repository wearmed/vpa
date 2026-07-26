package main

import (
	"fmt"
	"strings"
	"time"

	"vpa/internal/aurapi"
	"vpa/internal/ui"
)

// aurHealth describes what's worrying about an AUR package, if anything.
//
// Nobody vets the AUR. A package whose maintainer walked away, or that's
// been flagged out of date and left that way, is the one most likely to
// pull a dead source URL, fail its checksums, or build something older than
// it claims -- so it's worth saying so before the build starts rather than
// after it fails.
type aurHealth struct {
	Orphaned  bool
	OutOfDate bool
	Stale     bool
	Age       time.Duration
}

func (h aurHealth) any() bool { return h.Orphaned || h.OutOfDate || h.Stale }

// reasons lists what's wrong, in the order a person would care about it.
func (h aurHealth) reasons() []string {
	var out []string
	if h.Orphaned {
		out = append(out, "it has no maintainer (orphaned)")
	}
	if h.OutOfDate {
		out = append(out, "it's flagged out of date")
	}
	if h.Stale {
		out = append(out, fmt.Sprintf("it hasn't been updated in %s", humanAge(h.Age)))
	}
	return out
}

// checkAURHealth judges a package. staleDays of 0 turns off the age check
// entirely, for anyone who finds it noisy -- plenty of good AUR packages
// sit untouched for years simply because upstream hasn't released.
func checkAURHealth(p aurapi.Package, staleDays int, now time.Time) aurHealth {
	h := aurHealth{
		Orphaned:  p.Orphaned(),
		OutOfDate: p.OutOfDate != 0,
		Age:       p.StaleFor(now),
	}
	if staleDays > 0 && h.Age > time.Duration(staleDays)*24*time.Hour {
		h.Stale = true
	}
	return h
}

// humanAge renders a duration the way someone would say it out loud.
func humanAge(d time.Duration) string {
	days := int(d.Hours() / 24)
	switch {
	case days >= 730:
		return fmt.Sprintf("over %d years", days/365)
	case days >= 365:
		return "over a year"
	case days >= 60:
		return fmt.Sprintf("%d months", days/30)
	case days >= 30:
		return "over a month"
	default:
		return fmt.Sprintf("%d days", days)
	}
}

// aurHealthNote is the short suffix shown next to a search result.
func aurHealthNote(p aurapi.Package, staleDays int) string {
	h := checkAURHealth(p, staleDays, time.Now())
	var tags []string
	if h.Orphaned {
		tags = append(tags, "orphaned")
	}
	if h.OutOfDate {
		tags = append(tags, "out of date")
	}
	if h.Stale && !h.Orphaned && !h.OutOfDate {
		tags = append(tags, "unmaintained for "+humanAge(h.Age))
	}
	if len(tags) == 0 {
		return ""
	}
	return " [" + strings.Join(tags, ", ") + "]"
}

// confirmAURHealth is the second prompt for a package that looks neglected.
//
// It's deliberately separate from the build-script review: that one asks
// "do you trust this code", this one asks "is this package still alive".
// Someone can reasonably answer yes to the first and no to this. Like the
// script review, having "assume yes" configured isn't enough to skip it --
// that takes an explicit --noconfirm or TRUST_AUR=1.
func (a *App) confirmAURHealth(p aurapi.Package) error {
	h := checkAURHealth(p, a.Cfg.StaleDays, time.Now())
	if !h.any() {
		return nil
	}

	ui.Warn("'%s' looks neglected: %s.", p.Name, joinReasons(h.reasons()))
	switch {
	case h.Orphaned:
		ui.Info("An orphaned package has nobody fixing it when its sources move or upstream changes, so it may simply fail to build.")
	case h.OutOfDate:
		ui.Info("Someone reported this is behind upstream and it hasn't been updated since, so you may get an older version than you expect.")
	default:
		ui.Info("That's often fine -- plenty of packages just track software that hasn't released lately -- but it can also mean nobody's looking after it anymore.")
	}

	if a.ExplicitYes || a.Cfg.TrustAUR {
		return nil
	}
	if !ui.ConfirmAlways("Install '%s' anyway?", p.Name) {
		return fmt.Errorf("cancelled -- '%s' was not installed", p.Name)
	}
	return nil
}

// joinReasons renders a list as "a, b and c".
func joinReasons(r []string) string {
	switch len(r) {
	case 0:
		return ""
	case 1:
		return r[0]
	default:
		return strings.Join(r[:len(r)-1], ", ") + " and " + r[len(r)-1]
	}
}
