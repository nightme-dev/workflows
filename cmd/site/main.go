// Command site generates the source of the workflow index site.
//
// The site is a Jekyll site: this program writes the facts the site
// is built from — the workflows of the repository and the changes of
// the git history — into the data directory of the site source, and
// writes one page per workflow, one per namespace and the two index
// pages. A page carries nothing but front matter; the layouts of the
// templates directory do the rest.
//
// The templates are the only source of the layout, so a change of the
// layout is a change of the templates and of nothing else.
//
// The exit code is 0 when the site source was written, 8 when the
// repository does not fit the site, 9 when a path cannot be read, and
// 1 when the command was called wrong. The codes are the ones
// cmd/validate uses, so a CI job can branch on them the same way.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// The exit codes. They are the codes of cmd/validate, so a CI job
// can branch on them without knowing this program.
const (
	// exitOK says the site source was written.
	exitOK = 0
	// exitInvalid says the repository does not fit the site.
	exitInvalid = 8
	// exitUnreadable says a path does not exist or cannot be read.
	exitUnreadable = 9
	// exitUsage says the command was called wrong. It is nightme's
	// generic error code.
	exitUsage = 1
)

// The defaults of the flags. They are the paths this repository
// uses, so the command needs no arguments to do its work.
const (
	defaultTemplates    = "templates"
	defaultWorkflows    = "workflows"
	defaultOut          = "docs"
	defaultChangesLimit = 30
)

// invalidError says the repository does not fit the site.
//
// It is a property of the content, not of the paths: the site cannot
// show what the repository holds.
type invalidError struct {
	reason string
}

// Error implements the error interface.
func (e *invalidError) Error() string {
	return e.reason
}

// unreadableError says a path does not exist or cannot be read.
type unreadableError struct {
	reason string
}

// Error implements the error interface.
func (e *unreadableError) Error() string {
	return e.reason
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run generates the site source and returns the exit code.
//
// What was written goes to out; what could not be done goes to
// errOut. It is a function of its own so a test can call it without
// ending the process.
func run(args []string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("site", flag.ContinueOnError)
	flags.SetOutput(errOut)
	flags.Usage = func() {
		fmt.Fprintf(errOut, "usage: site [--templates <dir>] [--workflows <dir>] [--out <dir>] [--changes-limit <n>]\n")
	}
	templates := flags.String("templates", defaultTemplates, "the directory the site templates live in")
	workflows := flags.String("workflows", defaultWorkflows, "the directory the workflows live in")
	outDir := flags.String("out", defaultOut, "the directory the site source is written to")
	limit := flags.Int("changes-limit", defaultChangesLimit, "how many commits the changes page holds")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if flags.NArg() > 0 {
		flags.Usage()
		return exitUsage
	}
	if *limit < 0 {
		fmt.Fprintln(errOut, "site: --changes-limit cannot be negative")
		return exitUsage
	}
	data, err := build(*templates, *workflows, *limit)
	if err != nil {
		fmt.Fprintf(errOut, "site: %v\n", err)
		return codeOf(err)
	}
	if err := writeSite(*outDir, *templates, *workflows, data); err != nil {
		fmt.Fprintf(errOut, "site: %v\n", err)
		return codeOf(err)
	}
	fmt.Fprintf(out, "%s: %s, %s\n", *outDir,
		plural(len(data.Records), "workflow", "workflows"),
		plural(len(data.Changes), "change", "changes"))
	return exitOK
}

// usageError says the command was called wrong.
type usageError struct {
	reason string
}

// Error implements the error interface.
func (e *usageError) Error() string {
	return e.reason
}

// plural names a number of things.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// codeOf returns the exit code an error ends the run with.
func codeOf(err error) int {
	var invalid *invalidError
	if errors.As(err, &invalid) {
		return exitInvalid
	}
	var usage *usageError
	if errors.As(err, &usage) {
		return exitUsage
	}
	return exitUnreadable
}
