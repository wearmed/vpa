package ui

import (
	"strings"
	"testing"
)

func TestTrimNewline(t *testing.T) {
	cases := []struct{ in, want string }{
		{"y\n", "y"},
		{"y\r\n", "y"},
		{"y", "y"},
		{"\n", ""},
		{"", ""},
		{"multi\n\n", "multi"},
	}
	for _, c := range cases {
		if got := trimNewline(c.in); got != c.want {
			t.Errorf("trimNewline(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSetColorsToggles(t *testing.T) {
	SetColors("yes")
	if Bold("x") == "x" {
		t.Error(`--color=yes should emit escape codes`)
	}
	SetColors("no")
	if Bold("x") != "x" {
		t.Errorf(`--color=no should emit none, got %q`, Bold("x"))
	}
	// "auto" in a test binary has no tty, so it must behave like "no".
	SetColors("auto")
	if Bold("x") != "x" {
		t.Errorf("auto without a tty should not colorize, got %q", Bold("x"))
	}
}

// Messages go to stderr and results to stdout, and the two get redirected
// independently. Gating both on stderr meant `vpa search foo | grep bar`
// received escape codes, so output that clearly read "void/firefox" matched
// nothing.
func TestSetColorsGatesStreamsSeparately(t *testing.T) {
	defer SetColors("no")

	SetColors("no")
	if Bold("x") != "x" {
		t.Errorf("--color=no still emitted codes: %q", Bold("x"))
	}
	if colorRed != "" || colorReset != "" {
		t.Error("--color=no left message colours set")
	}

	SetColors("yes")
	if Bold("x") == "x" {
		t.Error("--color=yes produced no bold codes")
	}
	if colorRed == "" {
		t.Error("--color=yes left message colours unset")
	}
}

// Bold pairs with its own reset. Sharing colorReset meant that when stdout
// was piped and stderr was not, Bold emitted a reset with no opening code.
func TestBoldIsSelfContained(t *testing.T) {
	defer SetColors("no")

	SetColors("no")
	if got := Bold("firefox"); got != "firefox" {
		t.Errorf("Bold with colour off = %q, want bare text", got)
	}

	SetColors("yes")
	got := Bold("firefox")
	if !strings.HasPrefix(got, "\x1b[") || !strings.HasSuffix(got, "\x1b[0m") {
		t.Errorf("Bold with colour on = %q, want wrapped in codes", got)
	}
}
