package main

import (
	"fmt"
	"os"
	"strings"

	"vpa/internal/flatpak"
	"vpa/internal/ui"
)

// resolveFlatpakNames turns whatever the user typed into real Flatpak app
// IDs. `flatpak install` only accepts full reverse-DNS IDs -- a plain name
// like "obs-studio" fails outright with "Only last name segment can contain
// -" -- so anything that isn't already an ID gets looked up, with a
// numbered picker when the match isn't obvious.
func resolveFlatpakNames(names []string) ([]string, error) {
	var out []string
	for _, name := range names {
		if flatpak.LooksLikeAppID(name) {
			out = append(out, name)
			continue
		}
		id, err := resolveOneFlatpakName(name)
		if err != nil {
			return nil, err
		}
		if id != "" {
			out = append(out, id)
		}
	}
	return out, nil
}

func resolveOneFlatpakName(name string) (string, error) {
	apps, _ := flatpak.Search(name)
	if len(apps) == 0 {
		return "", fmt.Errorf("nothing on Flathub matches %q", name)
	}

	if id, remaining := narrowFlatpakMatches(name, apps); id != "" {
		return id, nil
	} else {
		apps = remaining
	}

	// Otherwise show what matched and let the user pick, same numbered
	// style as the AUR picker.
	fmt.Fprintf(os.Stderr, "\nFlathub matches for %q:\n", name)
	for i := len(apps) - 1; i >= 0; i-- {
		fmt.Fprintf(os.Stderr, "%s %s\n", ui.Bold(fmt.Sprintf("%2d", i+1)), apps[i].ID)
		if apps[i].Desc != "" {
			fmt.Fprintf(os.Stderr, "    %s\n", apps[i].Desc)
		}
	}
	if ui.NoConfirm {
		return "", fmt.Errorf("%q matches %d Flatpaks -- give the full app ID (e.g. %s)", name, len(apps), apps[0].ID)
	}
	fmt.Fprint(os.Stderr, "which one? (number, enter to cancel): ")
	line := readLine()
	idx, _ := parseSelection(line, len(apps))
	if len(idx) == 0 {
		return "", fmt.Errorf("nothing selected for %q", name)
	}
	return apps[idx[0]-1].ID, nil
}

// narrowFlatpakMatches picks the one app a name unambiguously means, or
// returns the shortlist to choose from.
//
// Auto-selecting requires the exact match on the ID's last segment to be
// unique: Flathub has five apps ending in ".calculator" and four in
// ".Browser", so taking the first would silently install an arbitrary one.
func narrowFlatpakMatches(name string, apps []flatpak.App) (id string, shortlist []flatpak.App) {
	lower := strings.ToLower(name)
	var exact []flatpak.App
	for _, a := range apps {
		seg := a.ID
		if i := strings.LastIndex(seg, "."); i >= 0 {
			seg = seg[i+1:]
		}
		if strings.ToLower(seg) == lower {
			exact = append(exact, a)
		}
	}
	switch {
	case len(exact) == 1:
		return exact[0].ID, nil
	case len(exact) > 1:
		return "", exact // narrow the choice to the plausible ones
	case len(apps) == 1:
		return apps[0].ID, nil
	}
	return "", apps
}
