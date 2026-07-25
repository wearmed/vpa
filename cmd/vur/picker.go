package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"vur/internal/aurapi"
	"vur/internal/ui"
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
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}

	var sel []string
	for _, tok := range strings.Fields(line) {
		if m := rangeRe.FindStringSubmatch(tok); m != nil {
			lo, _ := strconv.Atoi(m[1])
			hi, _ := strconv.Atoi(m[2])
			for j := lo; j <= hi; j++ {
				if j >= 1 && j <= n {
					sel = append(sel, pkgs[j-1].Name)
				}
			}
			continue
		}
		if idx, err := strconv.Atoi(tok); err == nil && idx >= 1 && idx <= n {
			sel = append(sel, pkgs[idx-1].Name)
		}
	}
	return sel, nil
}
