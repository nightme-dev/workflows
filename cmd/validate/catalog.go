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
// next to the workflows directory. It is a table: one row per
// workflow, the address in backticks in the first column.
var (
	catalogHeading = regexp.MustCompile(`(?i)^##\s+workflows\s*$`)
	sectionEnd     = regexp.MustCompile(`^#{1,2}\s`)
	codeSpan       = regexp.MustCompile("`([^`]+)`")
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
		ns, name, ok := strings.Cut(e.address, "/")
		if !ok || !nsPattern.MatchString(ns) || !namePattern.MatchString(name) {
			diags = append(diags, problem(path, e.line, 0,
				"the catalog entry %q is not a valid address", e.address))
			continue
		}
		valid[e.address] = true
	}
	// The tree, by address. The folded addresses are the ones whose
	// name does not have the shape of a name: a workflow is there,
	// but the name is wrong, and that is reported on its own.
	tree := map[string]bool{}
	folded := map[string]bool{}
	for _, b := range bundles {
		tree[b.address()] = true
		folded[strings.ToLower(b.address())] = true
	}
	reported := map[string]bool{}
	for _, e := range entries {
		if !valid[e.address] || reported[e.address] {
			continue
		}
		reported[e.address] = true
		if tree[e.address] {
			continue
		}
		if folded[strings.ToLower(e.address)] {
			// A workflow with that address is there; its name is
			// wrong, and that is the problem.
			continue
		}
		diags = append(diags, problem(path, e.line, 0,
			"the catalog lists %q, which no workflow has", e.address))
	}
	for _, b := range bundles {
		if listed[b.address()] {
			continue
		}
		if !nsPattern.MatchString(b.ns) || !namePattern.MatchString(b.name) {
			// The name is wrong: an address that cannot be listed
			// cannot be missing from the catalog.
			continue
		}
		diags = append(diags, problem(b.dir, 0, 0,
			"the workflow %q is not listed in the README catalog", b.address()))
	}
	return diags
}

// readCatalog returns the addresses the catalog of the README lists.
//
// The catalog is a table in the `## Workflows` section: one row per
// workflow, the address in backticks in the first column. Only that
// column is read, so neither the prose of the section nor the other
// columns of the table can add an entry to the catalog.
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
		cell, ok := firstCell(lines[i])
		if !ok {
			continue
		}
		address, ok := addressOf(cell)
		if !ok {
			continue
		}
		entries = append(entries, catalogEntry{address: address, line: i + 1})
	}
	return entries, true, nil
}

// firstCell returns the first cell of a row of a table.
//
// A line that is not a row of a table has no cell.
func firstCell(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") {
		return "", false
	}
	cells := strings.Split(strings.Trim(trimmed, "|"), "|")
	if len(cells) == 0 {
		return "", false
	}
	return cells[0], true
}

// addressOf returns the address a cell of the catalog holds.
//
// The cell holds the address in backticks. A cell that holds no
// backticked token holds no entry: the header of the table and its
// separator hold none either.
func addressOf(cell string) (string, bool) {
	m := codeSpan.FindStringSubmatch(cell)
	if m == nil {
		return "", false
	}
	token := strings.TrimSpace(m[1])
	if !strings.Contains(token, "/") {
		return "", false
	}
	return token, true
}
