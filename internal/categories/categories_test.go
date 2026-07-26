package categories

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseBasics(t *testing.T) {
	s := parse(`
# a comment
browser|browsers|web @WebBrowser: firefox chromium
browser: lynx firefox

games: steam
`)
	c, ok := s.Lookup("browser")
	if !ok {
		t.Fatal("browser not found")
	}
	if c.Freedesktop != "WebBrowser" {
		t.Errorf("Freedesktop = %q, want WebBrowser", c.Freedesktop)
	}
	// A repeated category adds to it, and a name already present isn't
	// duplicated.
	want := []string{"firefox", "chromium", "lynx"}
	if len(c.Packages) != len(want) {
		t.Fatalf("Packages = %v, want %v", c.Packages, want)
	}
	for i, w := range want {
		if c.Packages[i] != w {
			t.Errorf("Packages[%d] = %q, want %q", i, c.Packages[i], w)
		}
	}
	for _, alias := range []string{"browsers", "web"} {
		if got, ok := s.Lookup(alias); !ok || got != c {
			t.Errorf("alias %q did not resolve to browser", alias)
		}
	}
}

func TestParseSkipsMalformedLines(t *testing.T) {
	s := parse("this line has no colon\nempty:\n: nothing\ngames: steam\n")
	if _, ok := s.Lookup("games"); !ok {
		t.Error("a malformed line took out the rest of the file")
	}
	if len(s.Names()) != 1 {
		t.Errorf("Names() = %v, want just [games]", s.Names())
	}
}

func TestLookupIsCaseInsensitiveAndPrefixed(t *testing.T) {
	s := parse("browser: firefox\ngames: steam\n")
	if _, ok := s.Lookup("BROWSER"); !ok {
		t.Error("lookup should be case-insensitive")
	}
	if c, ok := s.Lookup("brow"); !ok || c.Name != "browser" {
		t.Error("a unique prefix should resolve")
	}
	if _, ok := s.Lookup("nonsense"); ok {
		t.Error("unknown name resolved")
	}
}

// An ambiguous prefix must not silently pick one, the same reasoning as the
// ambiguous --flatpak name fix: guessing installs the wrong thing.
func TestLookupRejectsAmbiguousPrefix(t *testing.T) {
	s := parse("games: steam\ngraphics: gimp\n")
	if c, ok := s.Lookup("g"); ok {
		t.Errorf("ambiguous prefix resolved to %q", c.Name)
	}
}

func TestMergeReplacesRatherThanAppends(t *testing.T) {
	s := parse("browser: firefox chromium lynx\ngames: steam\n")
	s.merge(parse("browser: firefox\n"))

	c, _ := s.Lookup("browser")
	if len(c.Packages) != 1 || c.Packages[0] != "firefox" {
		t.Errorf("Packages = %v, want just [firefox]", c.Packages)
	}
	if _, ok := s.Lookup("games"); !ok {
		t.Error("merge dropped an untouched category")
	}
}

func TestMergeRewiresAliases(t *testing.T) {
	s := parse("browser|web: firefox\n")
	s.merge(parse("browser|www: chromium\n"))

	if _, ok := s.Lookup("web"); ok {
		t.Error("stale alias from the replaced category still resolves")
	}
	c, ok := s.Lookup("www")
	if !ok || c.Packages[0] != "chromium" {
		t.Error("new alias did not resolve to the replacing category")
	}
}

func TestMergeAddsNewCategories(t *testing.T) {
	s := parse("browser: firefox\n")
	s.merge(parse("mine|custom: hello\n"))
	if c, ok := s.Lookup("custom"); !ok || c.Name != "mine" {
		t.Error("a category only in the overlay was not added")
	}
}

// The list compiled into vpa has to actually parse, and has to keep the
// categories the help text tells people to try.
func TestBuiltinConfIsUsable(t *testing.T) {
	s := parse(builtinConf)
	if len(s.Names()) < 20 {
		t.Fatalf("only %d built-in categories", len(s.Names()))
	}
	for _, name := range []string{"browser", "games", "development", "social", "entertainment", "dewm", "editor"} {
		c, ok := s.Lookup(name)
		if !ok {
			t.Errorf("built-in category %q is missing", name)
			continue
		}
		if len(c.Packages) == 0 {
			t.Errorf("built-in category %q is empty", name)
		}
	}
	if c, _ := s.Lookup("browser"); c.Freedesktop != "WebBrowser" {
		t.Errorf("browser.Freedesktop = %q, want WebBrowser", c.Freedesktop)
	}
}

func TestLoadPrecedence(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache.conf")
	user := filepath.Join(dir, "user.conf")

	// Missing files are the normal case, not an error.
	if len(Load(cache, user).Names()) == 0 {
		t.Fatal("Load with no overlay files returned nothing")
	}

	os.WriteFile(cache, []byte("browser: fromcache\ncacheonly: x\n"), 0o644)
	os.WriteFile(user, []byte("browser: fromuser\n"), 0o644)

	s := Load(cache, user)
	if c, _ := s.Lookup("browser"); c.Packages[0] != "fromuser" {
		t.Errorf("browser = %v, want the user's file to win", c.Packages)
	}
	if _, ok := s.Lookup("cacheonly"); !ok {
		t.Error("category from the cached copy was lost")
	}
}

func TestNeedsRefresh(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.conf")
	if !NeedsRefresh(missing) {
		t.Error("a missing cache should need refreshing")
	}
	fresh := filepath.Join(dir, "fresh.conf")
	os.WriteFile(fresh, []byte("browser: firefox\n"), 0o644)
	if NeedsRefresh(fresh) {
		t.Error("a just-written cache should not need refreshing")
	}
}
