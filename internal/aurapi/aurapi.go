// Package aurapi is a client for the AUR RPC (https://aur.archlinux.org/rpc/v5).
package aurapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"vpa/internal/config"
)

type Package struct {
	Name         string
	PackageBase  string
	Version      string
	Description  string
	URL          string
	License      []string
	Depends      []string
	MakeDepends  []string
	OptDepends   []string
	Maintainer   string
	NumVotes     int
	Popularity   float64
	LastModified int64
}

type rpcResponse struct {
	ResultCount int       `json:"resultcount"`
	Type        string    `json:"type"`
	Error       string    `json:"error"`
	Results     []Package `json:"results"`
}

var httpClient = &http.Client{Timeout: 20 * time.Second}

// get performs an HTTP GET with a few retries to smooth over the transient
// TLS/connection blips the AUR RPC occasionally throws (seen in practice).
func get(fullURL string) (*rpcResponse, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
		resp, err := httpClient.Get(fullURL)
		if err != nil {
			lastErr = err
			continue
		}
		var parsed rpcResponse
		err = json.NewDecoder(resp.Body).Decode(&parsed)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if parsed.Type == "error" {
			return nil, fmt.Errorf("AUR RPC error: %s", parsed.Error)
		}
		return &parsed, nil
	}
	return nil, fmt.Errorf("AUR RPC request failed after retries: %w", lastErr)
}

// Search runs a name-desc search for term.
func Search(term string) ([]Package, error) {
	u := fmt.Sprintf("%s/search?by=name-desc&arg=%s", config.AURRPC, url.QueryEscape(term))
	r, err := get(u)
	if err != nil {
		return nil, err
	}
	return r.Results, nil
}

// Info does a batched multiinfo lookup for one or more package names.
func Info(names ...string) ([]Package, error) {
	if len(names) == 0 {
		return nil, nil
	}
	var b strings.Builder
	b.WriteString(config.AURRPC)
	b.WriteString("/info?")
	for i, n := range names {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString("arg[]=")
		b.WriteString(url.QueryEscape(n))
	}
	r, err := get(b.String())
	if err != nil {
		return nil, err
	}
	return r.Results, nil
}

// PackageBase resolves the PackageBase for a single AUR package name, or ""
// if not found.
func PackageBase(name string) (string, error) {
	pkgs, err := Info(name)
	if err != nil {
		return "", err
	}
	for _, p := range pkgs {
		if p.Name == name {
			return p.PackageBase, nil
		}
	}
	return "", nil
}

// ByName finds a package by exact Name within a batch of Info results.
func ByName(pkgs []Package, name string) (Package, bool) {
	for _, p := range pkgs {
		if p.Name == name {
			return p, true
		}
	}
	return Package{}, false
}

// pkgNameRe matches the character set the AUR itself allows in package
// names. Names from the RPC become directory components under the build
// cache, so anything containing a path separator or ".." has to be
// rejected outright rather than trusted to be well-behaved.
var pkgNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9@._+-]*$`)

// ValidPackageName reports whether name is safe to use as a package name
// and as a single path component.
func ValidPackageName(name string) bool {
	if name == "" || len(name) > 255 || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, "/\\") {
		return false
	}
	return pkgNameRe.MatchString(name)
}
