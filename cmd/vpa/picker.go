package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"vpa/internal/aurapi"
	"vpa/internal/ui"
)

var rangeRe = regexp.MustCompile(`^([0-9]+)-([0-9]+)$`)

// interactiveSelectPkg is a numbered AUR search picker: prints results
// sorted so the most popular is the highest (easiest to reach) number,
// and accepts space-separated indices and/or ranges, e.g. "1 3 5-7".
func interactiveSelectPkg(term string) ([]string, error) {
	pkgs, err := aurapi.Search(term)
	if err != nil {
		return nil, err
	}
	if len(pkgs) == 0 {
		ui.Warn("no AUR results for '%s'", term)
		return nil, nil
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Popularity < pkgs[j].Popularity })

	n := len(pkgs)
	for i := n - 1; i >= 0; i-- {
		p := pkgs[i]
		fmt.Fprintf(os.Stderr, "%s aur/%s %s\n", ui.Bold(fmt.Sprintf("%2d", i+1)), p.Name, p.Version)
		if p.Description != "" {
			fmt.Fprintf(os.Stderr, "    %s\n", p.Description)
		}
	}

	fmt.Fprint(os.Stderr, "packages to install (e.g. '1 3 5-7', enter to cancel): ")
	line := readLine()
	if line == "" {
		return nil, nil
	}

	indices, invalid := parseSelection(line, n)
	for _, bad := range invalid {
		ui.Warn("ignoring %q -- expected a number from 1 to %d, or a range like 2-5", bad, n)
	}
	var sel []string
	for _, i := range indices {
		sel = append(sel, pkgs[i-1].Name)
	}
	return sel, nil
}

// parseSelection turns a picker response ("1 3 5-7") into 1-based indices
// within 1..n, in the order given and without duplicates. Anything that
// isn't a usable number or range is returned separately so the caller can
// tell the user rather than silently dropping it.
func parseSelection(line string, n int) (indices []int, invalid []string) {
	seen := make(map[int]bool)
	add := func(i int) {
		if i >= 1 && i <= n && !seen[i] {
			seen[i] = true
			indices = append(indices, i)
		}
	}

	for _, tok := range strings.Fields(line) {
		if m := rangeRe.FindStringSubmatch(tok); m != nil {
			lo, err1 := strconv.Atoi(m[1])
			hi, err2 := strconv.Atoi(m[2])
			if err1 != nil || err2 != nil {
				invalid = append(invalid, tok) // absurdly long digit strings overflow
				continue
			}
			if lo > hi {
				lo, hi = hi, lo // "5-2" clearly means the same span as "2-5"
			}
			if hi < 1 || lo > n {
				invalid = append(invalid, tok) // entirely outside the list
				continue
			}
			if lo < 1 {
				lo = 1
			}
			if hi > n {
				hi = n // a typo'd upper bound shouldn't spin over a huge range
			}
			for j := lo; j <= hi; j++ {
				add(j)
			}
			continue
		}
		idx, err := strconv.Atoi(tok)
		if err != nil || idx < 1 || idx > n {
			invalid = append(invalid, tok)
			continue
		}
		add(idx)
	}
	return indices, invalid
}

// readLine reads one trimmed line from stdin, shared by the AUR and
// Flathub pickers.
func readLine() string {
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}
