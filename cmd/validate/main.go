// Command validate checks the workflows of this repository.
//
// It is the machine half of the community process: it decides the
// objective facts — does the file parse, is it a workflow, are the
// names right, is every action wired, does the catalog agree with the
// tree — and it leaves the judgment to the reviewer.
//
// Every problem is one line, "path:line:column: message", the format
// of wfe.Diagnostic.Error(). A run where every workflow is valid
// prints nothing.
//
// The exit code is 0 when everything is valid, 8 when a workflow does
// not validate or a repository convention is broken, and 9 when a
// path cannot be read — the same codes as `nightme workflow validate`.
package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/cnlangzi/nightme/pkg/wfe"
)

// The exit codes. They are the codes `nightme workflow validate`
// uses, so a CI job can branch on them without knowing this program.
const (
	// exitOK says everything is valid.
	exitOK = 0
	// exitInvalid says a workflow does not validate or a repository
	// convention is broken.
	exitInvalid = 8
	// exitUnreadable says a path does not exist or cannot be read.
	exitUnreadable = 9
	// exitUsage says the command was called wrong. It is nightme's
	// generic error code.
	exitUsage = 1
)

// defaultRoot is the directory the workflows live in.
const defaultRoot = "workflows"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run validates the repository and returns the exit code.
//
// The problems go to out, one per line; what could not be read goes
// to errOut. It is a function of its own so a test can call it
// without ending the process.
func run(args []string, out, errOut io.Writer) int {
	if len(args) > 1 || (len(args) == 1 && strings.TrimSpace(args[0]) == "") {
		fmt.Fprintf(errOut, "usage: validate [%s]\n", defaultRoot)
		return exitUsage
	}
	root := defaultRoot
	if len(args) == 1 {
		root = args[0]
	}
	diags, err := validate(root)
	if err != nil {
		fmt.Fprintf(errOut, "%s: %v\n", root, err)
		return exitUnreadable
	}
	for _, d := range diags {
		fmt.Fprintln(out, d.Error())
	}
	if len(diags) > 0 {
		return exitInvalid
	}
	return exitOK
}

// validate checks the workflows under root and returns every problem
// it found.
//
// The error it returns is not a problem of a workflow: it says the
// path could not be read, and the caller turns it into the exit code
// 9. Every other problem is a diagnostic.
func validate(root string) ([]wfe.Diagnostic, error) {
	root = clean(root)
	bundles, diags, err := discover(root)
	if err != nil {
		return nil, err
	}
	// The repository uses the .yml extension only.
	diags = append(diags, checkExtensions(root)...)
	// The shape of every bundle, and the wiring of its actions.
	for _, b := range bundles {
		diags = append(diags, checkBundle(b)...)
	}
	// The shape of the names.
	diags = append(diags, checkNames(bundles)...)
	// The identity of every workflow is unique.
	diags = append(diags, checkIdentities(bundles)...)
	// An action name is shipped by one bundle.
	diags = append(diags, checkActionNames(bundles)...)
	// The catalog agrees with the tree. A single bundle carries no
	// address, so there is nothing to compare.
	if !isSingleBundle(bundles) {
		diags = append(diags, checkCatalog(root, bundles)...)
	}
	sortDiagnostics(diags)
	return diags, nil
}

// isSingleBundle reports whether the validator was pointed at one
// bundle rather than at the workflows directory.
//
// A single bundle carries no address of its own: its namespace and
// its name are read from the tree only when the whole tree is there.
func isSingleBundle(bundles []bundle) bool {
	return len(bundles) == 1 && bundles[0].address() == ""
}

// problem returns the diagnostic of a problem at a place.
//
// A place that is not there — a problem that belongs to no line of a
// file — gives a diagnostic without a position, the way wfe does.
func problem(path string, line, column int, format string, args ...any) wfe.Diagnostic {
	return wfe.Diagnostic{
		Path:    path,
		Line:    line,
		Column:  column,
		Message: fmt.Sprintf(format, args...),
	}
}

// sortDiagnostics orders the problems by path, then by place within
// the file: by line, then by column. A problem that has no place in
// the file comes last.
func sortDiagnostics(diags []wfe.Diagnostic) {
	sort.SliceStable(diags, func(i, j int) bool {
		a, b := diags[i], diags[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return position(a.Line) < position(b.Line)
		}
		return a.Column < b.Column
	})
}

// position returns the line of a problem for sorting. A problem that
// has no place in the file comes last.
func position(line int) int {
	if line <= 0 {
		return int(^uint(0) >> 1)
	}
	return line
}
