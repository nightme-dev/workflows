package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// writeSite writes the site source into the out directory.
//
// The directory is written from scratch: what a previous run left
// behind would show up as the pages of workflows that are gone.
func writeSite(out, templates, workflows string, data *siteData) error {
	if err := checkOut(out, templates, workflows); err != nil {
		return err
	}
	if err := os.RemoveAll(out); err != nil {
		return &unreadableError{reason: fmt.Sprintf("%s: %v", out, err)}
	}
	if err := copyTree(templates, out); err != nil {
		return err
	}
	if err := writeData(out, data); err != nil {
		return err
	}
	return writePages(out, data)
}

// checkOut refuses an output directory that holds the repository.
//
// The site source is written from scratch, so an output directory
// that holds the workflows or the templates would take them with it.
func checkOut(out, templates, workflows string) error {
	outAbs, err := filepath.Abs(out)
	if err != nil {
		return &unreadableError{reason: fmt.Sprintf("%s: %v", out, err)}
	}
	for _, keep := range []string{templates, workflows} {
		keepAbs, err := filepath.Abs(keep)
		if err != nil {
			continue
		}
		if outAbs == keepAbs || strings.HasPrefix(keepAbs, outAbs+string(filepath.Separator)) {
			return &usageError{reason: fmt.Sprintf("the output directory %s holds %s", out, keep)}
		}
	}
	return nil
}

// copyTree copies a directory into another one.
//
// The hidden files are left out: they are not part of the templates.
func copyTree(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return &unreadableError{reason: fmt.Sprintf("%s: %v", src, err)}
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return &unreadableError{reason: fmt.Sprintf("%s: %v", dst, err)}
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		from := filepath.Join(src, entry.Name())
		to := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyTree(from, to); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(from, to); err != nil {
			return err
		}
	}
	return nil
}

// copyFile copies a file.
func copyFile(from, to string) error {
	raw, err := os.ReadFile(from)
	if err != nil {
		return &unreadableError{reason: fmt.Sprintf("%s: %v", from, err)}
	}
	info, err := os.Stat(from)
	if err != nil {
		return &unreadableError{reason: fmt.Sprintf("%s: %v", from, err)}
	}
	if err := os.WriteFile(to, raw, info.Mode().Perm()); err != nil {
		return &unreadableError{reason: fmt.Sprintf("%s: %v", to, err)}
	}
	return nil
}

// writeData writes the facts the site is built from.
//
// The data is JSON: the templates read it, and the data is what
// another program would read too.
func writeData(out string, data *siteData) error {
	dir := filepath.Join(out, "_data")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &unreadableError{reason: fmt.Sprintf("%s: %v", dir, err)}
	}
	if err := writeJSON(filepath.Join(dir, "workflows.json"), data.Records); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, "changes.json"), data.Changes)
}

// writeJSON writes a value as JSON.
func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return &unreadableError{reason: fmt.Sprintf("%s: %v", path, err)}
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return &unreadableError{reason: fmt.Sprintf("%s: %v", path, err)}
	}
	return nil
}

// writePages writes the content pages of the site.
//
// A page holds nothing but front matter: the layout of the templates
// is what renders it.
func writePages(out string, data *siteData) error {
	pages := []page{
		{path: "index.md", fields: []string{"layout: home", "permalink: /"}},
		{path: "changes.md", fields: []string{"layout: changes", "permalink: /changes/"}},
	}
	for _, ns := range data.Namespaces {
		pages = append(pages, page{
			path:   ns + ".md",
			fields: []string{"layout: namespace", "namespace: " + ns, "permalink: /" + ns + "/"},
		})
	}
	for _, rec := range data.Records {
		pages = append(pages, page{
			path:   rec.Namespace + "/" + rec.Name + ".md",
			fields: []string{"layout: workflow", "address: " + rec.Address, "permalink: /" + rec.Address + "/"},
		})
	}
	for _, p := range pages {
		if err := writePage(filepath.Join(out, p.path), p.fields); err != nil {
			return err
		}
	}
	return nil
}

// page is one content page of the site.
type page struct {
	// path is the path of the page relative to the site source.
	path string
	// fields are the lines of the front matter of the page.
	fields []string
}

// writePage writes one page.
func writePage(path string, fields []string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &unreadableError{reason: fmt.Sprintf("%s: %v", dir, err)}
	}
	var b strings.Builder
	b.WriteString("---\n")
	for _, field := range fields {
		b.WriteString(field)
		b.WriteString("\n")
	}
	b.WriteString("---\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return &unreadableError{reason: fmt.Sprintf("%s: %v", path, err)}
	}
	return nil
}
