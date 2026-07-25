package main

import (
	"reflect"
	"testing"
)

func TestParseSelection(t *testing.T) {
	const n = 10
	cases := []struct {
		line        string
		want        []int
		wantInvalid []string
	}{
		{"1", []int{1}, nil},
		{"1 3 5", []int{1, 3, 5}, nil},
		{"2-5", []int{2, 3, 4, 5}, nil},
		{"1 3 5-7", []int{1, 3, 5, 6, 7}, nil},
		// Order given is preserved.
		{"7 2", []int{7, 2}, nil},
		// Duplicates collapse, whether repeated directly or via a range.
		{"1 1", []int{1}, nil},
		{"2-4 3", []int{2, 3, 4}, nil},
		{"3 2-4", []int{3, 2, 4}, nil},
		// A reversed range means the same span.
		{"5-2", []int{2, 3, 4, 5}, nil},
		// Bounds are clamped rather than panicking or spinning.
		{"1-99999", []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, nil},
		{"0-3", []int{1, 2, 3}, nil},
		// Entirely out of range, or not a number at all.
		{"0", nil, []string{"0"}},
		{"11", nil, []string{"11"}},
		{"abc", nil, []string{"abc"}},
		{"20-30", nil, []string{"20-30"}},
		{"-", nil, []string{"-"}},
		{"1 abc 3", []int{1, 3}, []string{"abc"}},
		{"", nil, nil},
		{"   ", nil, nil},
	}
	for _, c := range cases {
		got, invalid := parseSelection(c.line, n)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseSelection(%q) = %v, want %v", c.line, got, c.want)
		}
		if !reflect.DeepEqual(invalid, c.wantInvalid) {
			t.Errorf("parseSelection(%q) invalid = %v, want %v", c.line, invalid, c.wantInvalid)
		}
	}
}

// Indices must always be usable as pkgs[i-1] without going out of bounds.
func TestParseSelectionNeverOutOfBounds(t *testing.T) {
	for _, n := range []int{0, 1, 3} {
		for _, line := range []string{"1", "0", "5", "1-100", "100-1", "-5", "3-3", "abc"} {
			got, _ := parseSelection(line, n)
			for _, i := range got {
				if i < 1 || i > n {
					t.Errorf("parseSelection(%q, %d) returned out-of-range index %d", line, n, i)
				}
			}
		}
	}
}

// A huge digit string must not overflow into a valid-looking index.
func TestParseSelectionHugeNumbers(t *testing.T) {
	got, invalid := parseSelection("99999999999999999999999", 5)
	if len(got) != 0 {
		t.Errorf("expected no selection, got %v", got)
	}
	if len(invalid) != 1 {
		t.Errorf("expected the token reported as invalid, got %v", invalid)
	}
	got, _ = parseSelection("1-99999999999999999999999", 5)
	for _, i := range got {
		if i < 1 || i > 5 {
			t.Errorf("out-of-range index %d from an overflowing range", i)
		}
	}
}
