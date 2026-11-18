package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cnlangzi/nightme/pkg/wfe"
)

// The shape of the catalog in the README.
//
// The catalog is the `## Workflows` section of the README that sits
// next to the workflows directory. Every address of the section is
// written in backticks: `nightme/coder`.
var (
	catalogHeading = regexp.MustCompile(`(?i)^##\s+workflows\s*$`)
	sectionEnd     = regexp.MustCompile(`^#{1,2}\s`)
	codeSpan       = regexp.MustCompile("`([^`]+)`")
	addressPart    = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
)

// catalogEntry is one address of the catalog and the line it stands
// on.
type catalogEntry struct {
	address string
	line    int
}

// catalogPath returns the path of the README that holds the catalog.
func catalogPath(root string) string {
	return filepath.Join(filepath.Dir(root), "README.md")
}

// checkCatalog checks that the README catalog and the tree agree.
//
// The catalog is written by hand and the tree is the truth: a
// workflow that is not listed is invisible to a reader, and an entry
// that names no workflow is a promise the repository does not keep.
func checkCatalog(root string, bundles []bundle) []wfe.Diagnostic {
	path := catalogPath(root)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// No README next to the workflows: there is no catalog
			// to compare the tree with.
			return nil
		}
		return []wfe.Diagnostic{{Path: path, Message: err.Error()}}
	}
	entries, found, err := readCatalog(path)
	if err != nil {
		return []wfe.Diagnostic{{Path: path, Message: err.Error()}}
	}
	if !found {
		return []wfe.Diagnostic{{Path: path, Message: "the README has no `## Workflows` section (the workflow catalog)"}}
	}
	listed := map[string]bool{}
	valid := map[string]bool{}
	var diags []wfe.Diagnostic
	for _, e := range entries {
		if listed[e.address] {
			continue
		}
		listed[e.address] = true
		ns, name, _ := strings.Cut(e.address, "/")
		if !nsPattern.MatchString(ns) || !namePattern.MatchString(name) {
			diags = append(diags, problem(path, e.line, 0,
				"the catalog entry %q is not a valid address", e.address))
			continue
		}
		valid[e.address] = true
	}
	tree := map[string]bool{}
	for _, b := range bundles {
		tree[b.address()] = true
	}
	reported := map[string]bool{}
	for _, e := range entries {
		if !valid[e.address] || reported[e.address] {
			continue
		}
		reported[e.address] = true
		if !tree[e.address] {
			diags = append(diags, problem(path, e.line, 0,
				"the catalog lists %q, which no workflow has", e.address))
		}
	}
	for _, b := range bundles {
		if !listed[b.address()] {
			diags = append(diags, problem(b.dir, 0, 0,
				"the workflow %q is not listed in the README catalog", b.address()))
		}
	}
	return diags
}

// readCatalog returns the addresses the catalog of the README lists.
//
// The second value says whether the README has a catalog section at
// all: a README without one is a README that promises nothing.
func readCatalog(path string) ([]catalogEntry, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	lines := strings.Split(string(raw), "\n")
	start := -1
	for i, line := range lines {
		if catalogHeading.MatchString(line) {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, false, nil
	}
	var entries []catalogEntry
	for i := start + 1; i < len(lines); i++ {
		if sectionEnd.MatchString(lines[i]) {
			break
		}
		for _, m := range codeSpan.FindAllStringSubmatch(lines[i], -1) {
			address, ok := addressOf(m[1])
			if !ok {
				continue
			}
			entries = append(entries, catalogEntry{address: address, line: i + 1})
		}
	}
	return entries, true, nil
}

// addressOf returns the address a backticked token holds.
//
// A token that is not an address — a word, a path — is not one: only
// a token of two names is read as an address, so the prose around the
// catalog cannot add entries to it.
func addressOf(token string) (string, bool) {
	parts := strings.Split(token, "/")
	if len(parts) != 2 {
		return "", false
	}
	for _, part := range parts {
		if !addressPart.MatchString(part) {
			return "", false
		}
	}
	return token, true
}
