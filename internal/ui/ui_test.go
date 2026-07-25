package ui

import "testing"

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
