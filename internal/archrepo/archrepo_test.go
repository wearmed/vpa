package archrepo

import "testing"

func TestSoBaseName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"usr/lib/libXss.so.1", "libXss.so.1"},
		{"usr/lib64/libfoo.so.12", "libfoo.so.12"},
		{"libbare.so.3", "libbare.so.3"},
		{"", ""},
	}
	for _, c := range cases {
		if got := soBaseName(c.in); got != c.want {
			t.Errorf("soBaseName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The file-listing scrape must pick up real sonames (lib*.so.MAJOR) and
// skip both the unversioned development symlink and the fully-versioned
// real file, since Void's shlib-provides only ever records the middle form.
func TestSonameListingPattern(t *testing.T) {
	html := `<li class="d">usr/lib/</li>
<li class="f">usr/lib/libXss.so</li>
<li class="f">usr/lib/libXss.so.1</li>
<li class="f">usr/lib/libXss.so.1.0.0</li>
<li class="f">usr/lib/pkgconfig/xscrnsaver.pc</li>
<li class="f">usr/lib64/libother.so.7</li>
<li class="f">usr/include/X11/extensions/scrnsaver.h</li>`

	var got []string
	for _, m := range liSoRe.FindAllStringSubmatch(html, -1) {
		got = append(got, soBaseName(m[1]))
	}

	want := map[string]bool{"libXss.so.1": true, "libother.so.7": true}
	if len(got) != len(want) {
		t.Fatalf("matched %v, want exactly %v", got, want)
	}
	for _, g := range got {
		if !want[g] {
			t.Errorf("unexpected match %q", g)
		}
	}
}

func TestSonameListingIgnoresNonLibraries(t *testing.T) {
	html := `<li class="f">usr/bin/hello</li><li class="f">usr/share/doc/README.so.txt</li>`
	if m := liSoRe.FindAllStringSubmatch(html, -1); len(m) != 0 {
		t.Errorf("matched non-library paths: %v", m)
	}
}
