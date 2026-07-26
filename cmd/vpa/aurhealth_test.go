package main

import (
	"strings"
	"testing"
	"time"

	"vpa/internal/aurapi"
)

func daysAgo(n int) int64 { return time.Now().AddDate(0, 0, -n).Unix() }

func TestCheckAURHealth(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		pkg  aurapi.Package
		days int
		want aurHealth
	}{
		{
			name: "healthy",
			pkg:  aurapi.Package{Maintainer: "someone", LastModified: daysAgo(2)},
			days: 30,
		},
		{
			name: "orphaned",
			pkg:  aurapi.Package{Maintainer: "", LastModified: daysAgo(2)},
			days: 30,
			want: aurHealth{Orphaned: true},
		},
		{
			name: "flagged out of date",
			pkg:  aurapi.Package{Maintainer: "someone", LastModified: daysAgo(2), OutOfDate: daysAgo(1)},
			days: 30,
			want: aurHealth{OutOfDate: true},
		},
		{
			name: "stale",
			pkg:  aurapi.Package{Maintainer: "someone", LastModified: daysAgo(200)},
			days: 30,
			want: aurHealth{Stale: true},
		},
		{
			// A day either side of the threshold is the boundary that
			// actually decides whether someone gets prompted.
			name: "just inside the threshold",
			pkg:  aurapi.Package{Maintainer: "someone", LastModified: daysAgo(29)},
			days: 30,
		},
		{
			name: "staleness check disabled",
			pkg:  aurapi.Package{Maintainer: "someone", LastModified: daysAgo(2000)},
			days: 0,
		},
		{
			name: "everything at once",
			pkg:  aurapi.Package{Maintainer: "", LastModified: daysAgo(400), OutOfDate: daysAgo(300)},
			days: 30,
			want: aurHealth{Orphaned: true, OutOfDate: true, Stale: true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checkAURHealth(tc.pkg, tc.days, now)
			if got.Orphaned != tc.want.Orphaned || got.OutOfDate != tc.want.OutOfDate || got.Stale != tc.want.Stale {
				t.Errorf("got {orphaned:%v outofdate:%v stale:%v}, want {orphaned:%v outofdate:%v stale:%v}",
					got.Orphaned, got.OutOfDate, got.Stale,
					tc.want.Orphaned, tc.want.OutOfDate, tc.want.Stale)
			}
			if got.any() != (tc.want.Orphaned || tc.want.OutOfDate || tc.want.Stale) {
				t.Errorf("any() = %v", got.any())
			}
		})
	}
}

// A package with no LastModified at all (the AUR has sent 0 before) must not
// read as infinitely old and prompt on every install.
func TestCheckAURHealthMissingTimestamp(t *testing.T) {
	h := checkAURHealth(aurapi.Package{Maintainer: "someone"}, 30, time.Now())
	if h.Stale {
		t.Error("a package with no timestamp was judged stale")
	}
}

func TestAURHealthNote(t *testing.T) {
	cases := []struct {
		name string
		pkg  aurapi.Package
		want string
	}{
		{"healthy", aurapi.Package{Maintainer: "x", LastModified: daysAgo(1)}, ""},
		{"orphaned", aurapi.Package{LastModified: daysAgo(1)}, " [orphaned]"},
		{"out of date", aurapi.Package{Maintainer: "x", LastModified: daysAgo(1), OutOfDate: daysAgo(1)}, " [out of date]"},
		{"stale only", aurapi.Package{Maintainer: "x", LastModified: daysAgo(400)}, " [unmaintained for over a year]"},
		// Staleness is implied by the other two, so it isn't repeated.
		{"orphaned and stale", aurapi.Package{LastModified: daysAgo(400)}, " [orphaned]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := aurHealthNote(tc.pkg, 30); got != tc.want {
				t.Errorf("aurHealthNote() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHumanAge(t *testing.T) {
	day := 24 * time.Hour
	cases := []struct {
		days int
		want string
	}{
		{5, "5 days"},
		{29, "29 days"},
		{35, "over a month"},
		{90, "3 months"},
		{400, "over a year"},
		{800, "over 2 years"},
	}
	for _, tc := range cases {
		if got := humanAge(time.Duration(tc.days) * day); got != tc.want {
			t.Errorf("humanAge(%dd) = %q, want %q", tc.days, got, tc.want)
		}
	}
}

func TestJoinReasons(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a and b"},
		{[]string{"a", "b", "c"}, "a, b and c"},
	}
	for _, tc := range cases {
		if got := joinReasons(tc.in); got != tc.want {
			t.Errorf("joinReasons(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestReasonsCoverEveryFlag(t *testing.T) {
	h := checkAURHealth(aurapi.Package{LastModified: daysAgo(400), OutOfDate: daysAgo(1)}, 30, time.Now())
	joined := strings.Join(h.reasons(), "; ")
	for _, want := range []string{"orphaned", "out of date", "hasn't been updated"} {
		if !strings.Contains(joined, want) {
			t.Errorf("reasons() = %q, missing %q", joined, want)
		}
	}
}
