// Package archrepo is a read-only client for Arch Linux's public package
// database (archlinux.org), used as a last-resort dependency-resolution
// fallback: when a PKGBUILD dependency has no Void equivalent by name and
// isn't on the AUR either, we look up what shared libraries the real Arch
// package actually ships and check whether some differently-named Void
// package provides the same ones. That's a much safer translation than
// guessing from name similarity -- it matches on actual ABI, not spelling.
package archrepo

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"time"
)

type pkgInfo struct {
	Pkgname string `json:"pkgname"`
	Repo    string `json:"repo"`
	Arch    string `json:"arch"`
}

type searchResponse struct {
	Results []pkgInfo `json:"results"`
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// liSoRe matches a proper soname entry in Arch's file listing (e.g.
// "usr/lib/libXss.so.1") -- deliberately excludes the unversioned dev
// symlink ("libXss.so") and the fully-versioned real file
// ("libXss.so.1.0.0"), keeping just the ldconfig-style "lib<name>.so.<major>"
// form, which is what Void's own shlib-provides tracks too.
var liSoRe = regexp.MustCompile(`<li class="f">(usr/lib(?:64|32)?/lib[^<]+\.so\.[0-9]+)</li>`)

// Sonames looks up name in Arch's official repos and returns the sonames
// (e.g. "libXss.so.1") it actually ships, or nil if name isn't a real Arch
// package or ships no shared libraries.
func Sonames(name string) []string {
	info, ok := lookup(name)
	if !ok {
		return nil
	}
	return filesSonames(info)
}

func lookup(name string) (pkgInfo, bool) {
	resp, err := httpClient.Get("https://archlinux.org/packages/search/json/?name=" + name)
	if err != nil {
		return pkgInfo{}, false
	}
	defer resp.Body.Close()

	var parsed searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return pkgInfo{}, false
	}
	for _, p := range parsed.Results {
		if p.Pkgname == name {
			return p, true
		}
	}
	return pkgInfo{}, false
}

func filesSonames(info pkgInfo) []string {
	url := "https://archlinux.org/packages/" + info.Repo + "/" + info.Arch + "/" + info.Pkgname + "/files/"
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var out []string
	seen := make(map[string]bool)
	for _, m := range liSoRe.FindAllSubmatch(body, -1) {
		base := soBaseName(string(m[1]))
		if base != "" && !seen[base] {
			seen[base] = true
			out = append(out, base)
		}
	}
	return out
}

// soBaseName extracts just the filename (e.g. "libXss.so.1") from a full
// path (e.g. "usr/lib/libXss.so.1").
func soBaseName(path string) string {
	i := len(path) - 1
	for i >= 0 && path[i] != '/' {
		i--
	}
	return path[i+1:]
}
