package pkgbuild

import (
	"regexp"
	"strings"
)

// Summary is a plain-language overview of what a PKGBUILD will install,
// extracted by reading the file as *text* -- deliberately never executing
// it. It exists so a user can be shown something understandable before
// deciding whether to trust the script enough to run it, since the full
// text is a wall of bash that most people won't actually read.
//
// Being a text scrape, it can't resolve values built from shell variables
// or command substitution; those simply show as-written. It's for display
// only. Everything vpa actually builds with comes from Load(), which runs
// after the user has approved the script.
type Summary struct {
	Name        string
	Version     string
	Description string
	URL         string
	License     string
	Depends     []string
	Sources     []string
	HasInstall  bool // ships a .install scriptlet (runs code at install time)
}

var (
	// Horizontal whitespace only ([ \t], not \s): \s matches newlines in Go,
	// which lets an empty assignment like "install=" swallow the line break
	// and capture text from a later line entirely. (.*?) rather than (.+?)
	// so an empty value stays empty instead of failing to match at all.
	scalarRe = regexp.MustCompile(`(?m)^[ \t]*(pkgname|pkgver|pkgrel|pkgdesc|url|license|install)[ \t]*=[ \t]*(.*?)[ \t]*$`)
	arrayRe  = regexp.MustCompile(`(?ms)^[ \t]*(depends|source)[ \t]*=[ \t]*\((.*?)\)`)
)

// Summarize scrapes a PKGBUILD's text for display purposes.
func Summarize(text []byte) Summary {
	var s Summary
	src := string(text)

	var rel string
	for _, m := range scalarRe.FindAllStringSubmatch(src, -1) {
		val := unquote(m[2])
		switch m[1] {
		case "pkgname":
			s.Name = val
		case "pkgver":
			s.Version = val
		case "pkgrel":
			rel = val
		case "pkgdesc":
			s.Description = val
		case "url":
			s.URL = val
		case "license":
			// license is usually an array literal: license=('MIT')
			s.License = strings.Join(splitArray(strings.Trim(val, "()")), ", ")
		case "install":
			s.HasInstall = val != ""
		}
	}
	if rel != "" && s.Version != "" {
		s.Version += "-" + rel
	}

	for _, m := range arrayRe.FindAllStringSubmatch(src, -1) {
		items := splitArray(m[2])
		switch m[1] {
		case "depends":
			s.Depends = items
		case "source":
			s.Sources = items
		}
	}
	return s
}

func unquote(v string) string {
	v = strings.TrimSpace(v)
	// Drop a trailing inline comment outside quotes (common in PKGBUILDs).
	if i := strings.Index(v, " #"); i >= 0 && !strings.Contains(v[:i], "'") && !strings.Contains(v[:i], `"`) {
		v = strings.TrimSpace(v[:i])
	}
	v = strings.Trim(v, `"'`)
	return v
}

func splitArray(body string) []string {
	var out []string
	for _, f := range strings.Fields(body) {
		f = strings.Trim(f, `"'`)
		if f != "" && !strings.HasPrefix(f, "#") {
			out = append(out, f)
		}
	}
	return out
}

// SourceHosts returns the distinct hosts a PKGBUILD downloads from, so a
// user can see where the code is actually coming from at a glance.
func (s Summary) SourceHosts() []string {
	seen := map[string]bool{}
	var hosts []string
	for _, src := range s.Sources {
		_, url := SplitSourceEntry(src)
		url = strings.TrimPrefix(url, "git+")
		if !strings.Contains(url, "://") {
			continue
		}
		rest := url[strings.Index(url, "://")+3:]
		host := rest
		if i := strings.IndexAny(rest, "/#?"); i >= 0 {
			host = rest[:i]
		}
		if host != "" && !seen[host] {
			seen[host] = true
			hosts = append(hosts, host)
		}
	}
	return hosts
}
