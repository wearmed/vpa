// Package categories maps a plain-language category ("browser") to the
// packages that belong in it.
//
// Nothing on the xbps or AUR side records what kind of thing a package is,
// so the map is curated rather than derived. Flathub does record it, via
// AppStream, so a category can also carry a freedesktop category name that
// the Flatpak side is looked up from live.
package categories

import (
	"bufio"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

//go:embed categories.conf
var builtinConf string

// DefaultURL is where a newer category list is fetched from. It's served
// straight out of the repo, so publishing an update is a git push.
const DefaultURL = "https://git.wearmed.xyz/suraj/vpa/raw/branch/main/internal/categories/categories.conf"

// RefreshAfter is how stale the cached copy may get before it's re-fetched.
const RefreshAfter = 7 * 24 * time.Hour

// Category is one named group of packages.
type Category struct {
	Name string
	// Aliases are the other names this category answers to.
	Aliases []string
	// Freedesktop is the AppStream category (e.g. "WebBrowser") to look
	// the Flatpak side up from, empty if this category has no equivalent.
	Freedesktop string
	// Packages are candidate names: Void/AUR package names, and Flatpak
	// application IDs. Not all of them exist -- callers filter.
	Packages []string
}

// Set is a parsed category map.
type Set struct {
	byName map[string]*Category
	order  []*Category
}

// parse reads the conf format:
//
//	name|alias|alias @FreedesktopCategory: pkg pkg pkg
//
// Repeating a name adds to that category. Blank lines and #-comments are
// ignored. A malformed line is skipped rather than failing the whole file:
// one bad line in a user's config shouldn't take out every category.
func parse(text string) *Set {
	s := &Set{byName: map[string]*Category{}}
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		head, body, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		var freedesktop string
		if h, fd, ok := strings.Cut(head, "@"); ok {
			head, freedesktop = h, strings.TrimSpace(fd)
		}
		names := splitNames(head)
		pkgs := strings.Fields(body)
		if len(names) == 0 || len(pkgs) == 0 {
			continue
		}
		s.add(names, freedesktop, pkgs)
	}
	return s
}

func splitNames(head string) []string {
	var out []string
	for _, n := range strings.Split(head, "|") {
		n = strings.ToLower(strings.TrimSpace(n))
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

func (s *Set) add(names []string, freedesktop string, pkgs []string) {
	c := s.byName[names[0]]
	if c == nil {
		c = &Category{Name: names[0]}
		s.byName[names[0]] = c
		s.order = append(s.order, c)
	}
	if freedesktop != "" {
		c.Freedesktop = freedesktop
	}
	for _, alias := range names[1:] {
		if s.byName[alias] == nil {
			s.byName[alias] = c
			c.Aliases = append(c.Aliases, alias)
		}
	}
	seen := make(map[string]bool, len(c.Packages))
	for _, p := range c.Packages {
		seen[p] = true
	}
	for _, p := range pkgs {
		if !seen[p] {
			seen[p] = true
			c.Packages = append(c.Packages, p)
		}
	}
}

// merge overlays other onto s. A category redefined in other replaces the
// one in s outright rather than adding to it -- someone trimming the
// browser list in their own config means "this list", not "these as well".
func (s *Set) merge(other *Set) {
	for _, c := range other.order {
		if old, ok := s.byName[c.Name]; ok {
			for _, alias := range old.Aliases {
				delete(s.byName, alias)
			}
			*old = *c
			old.Name = c.Name
			for _, alias := range c.Aliases {
				s.byName[alias] = old
			}
			continue
		}
		s.byName[c.Name] = c
		s.order = append(s.order, c)
		for _, alias := range c.Aliases {
			s.byName[alias] = c
		}
	}
}

// Lookup finds a category by name or alias. An unrecognized name falls back
// to a unique prefix match, so "brows" finds "browser".
func (s *Set) Lookup(name string) (*Category, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if c, ok := s.byName[name]; ok {
		return c, true
	}
	var hit *Category
	for n, c := range s.byName {
		if strings.HasPrefix(n, name) {
			if hit != nil && hit != c {
				return nil, false
			}
			hit = c
		}
	}
	return hit, hit != nil
}

// Names lists every category name, alphabetically, without the aliases.
func (s *Set) Names() []string {
	out := make([]string, 0, len(s.order))
	for _, c := range s.order {
		out = append(out, c.Name)
	}
	sort.Strings(out)
	return out
}

// All returns every category, alphabetically.
func (s *Set) All() []*Category {
	out := append([]*Category(nil), s.order...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Load builds the category map from, in increasing order of precedence:
// the list compiled into vpa, the cached copy fetched from the server, and
// the user's own file. Any of the last two being missing or unreadable is
// normal, not an error.
func Load(cacheFile, userFile string) *Set {
	s := parse(builtinConf)
	if data, err := os.ReadFile(cacheFile); err == nil {
		s.merge(parse(string(data)))
	}
	if data, err := os.ReadFile(userFile); err == nil {
		s.merge(parse(string(data)))
	}
	return s
}

// NeedsRefresh reports whether the cached copy is missing or old enough to
// be worth re-fetching.
func NeedsRefresh(cacheFile string) bool {
	fi, err := os.Stat(cacheFile)
	if err != nil {
		return true
	}
	return time.Since(fi.ModTime()) > RefreshAfter
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// Refresh downloads the category list into cacheFile. The response has to
// parse into at least one category before it replaces what's already
// cached, so a login page or an error page served with a 200 can't quietly
// wipe out a working list.
func Refresh(url, cacheFile string) error {
	if url == "" {
		url = DefaultURL
	}
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if len(parse(string(data)).order) == 0 {
		return fmt.Errorf("%s: no categories in the response", url)
	}
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
		return err
	}
	tmp := cacheFile + ".new"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, cacheFile)
}
