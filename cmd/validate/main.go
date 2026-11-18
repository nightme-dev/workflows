// Command validate checks the workflows of this repository.
//
// It is the machine half of the community process: it decides the
// objective facts — does the file parse, is it a workflow, are the
// names right, is every action wired — and it leaves the judgment to
// the reviewer.
//
// It has two modes. The full mode checks the whole tree. The scoped
// mode checks only the bundles it is given, so that a pull request is
// judged on what it changed.
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
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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

// errNotRoot says the root is a bundle rather than the workflows
// directory. The scoped mode names bundles under the root, so it
// cannot run against a bundle.
var errNotRoot = errors.New("the root is a bundle, not the workflows directory")

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run validates the repository and returns the exit code.
//
// The problems go to out, one per line; what could not be read goes
// to errOut. It is a function of its own so a test can call it
// without ending the process.
func run(args []string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	flags.SetOutput(errOut)
	flags.Usage = func() {
		fmt.Fprintf(errOut, "usage: validate [--changed <values>] [%s]\n", defaultRoot)
	}
	var changed changedList
	flags.Var(&changed, "changed", "the bundles to validate")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	rest := flags.Args()
	if len(rest) > 1 || (len(rest) == 1 && strings.TrimSpace(rest[0]) == "") {
		flags.Usage()
		return exitUsage
	}
	root := defaultRoot
	if len(rest) == 1 {
		root = rest[0]
	}
	var diags []wfe.Diagnostic
	var err error
	if changed.given {
		diags, err = validateChanged(root, changed.values)
	} else {
		diags, err = validate(root)
	}
	if err != nil {
		if errors.Is(err, errNotRoot) {
			flags.Usage()
			return exitUsage
		}
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

// changedList is the value of the --changed flag: the bundles the
// scoped mode validates.
//
// The flag may be given more than once, and one value may hold more
// than one bundle: the bundles are separated by commas. The list
// remembers that the flag was given, so that an empty value is not
// the absence of the flag.
type changedList struct {
	given  bool
	values []string
}

// String returns the value of the flag, the way it is written on the
// command line.
func (c *changedList) String() string {
	return strings.Join(c.values, ",")
}

// Set adds the bundles of one value of the flag.
//
// An empty bundle is not a bundle: it is dropped, so that an empty
// value names nothing at all.
func (c *changedList) Set(value string) error {
	c.given = true
	for _, v := range strings.Split(value, ",") {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		c.values = append(c.values, v)
	}
	return nil
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
	extensions, err := checkExtensions(root)
	if err != nil {
		return nil, err
	}
	diags = append(diags, extensions...)
	// The shape of every bundle, and the wiring of its actions.
	for _, b := range bundles {
		bundleDiags, err := checkBundle(b)
		if err != nil {
			return nil, err
		}
		diags = append(diags, bundleDiags...)
	}
	// The shape of the names.
	diags = append(diags, checkNames(bundles)...)
	// The identity of every workflow is unique.
	diags = append(diags, checkIdentities(bundles)...)
	// An action name is shipped by one bundle.
	diags = append(diags, checkActionNames(bundles)...)
	sortDiagnostics(diags)
	return diags, nil
}

// validateChanged checks only the bundles the values name.
//
// The values are paths under the root: a bundle, a namespace
// directory or a file. The scoped mode consults nothing global: it
// reads the named bundles and nothing else, so a bundle stands on its
// own.
//
// The error it returns is not a problem of a workflow: it says the
// path could not be read, and the caller turns it into the exit code
// 9. Every other problem is a diagnostic.
func validateChanged(root string, values []string) ([]wfe.Diagnostic, error) {
	root = clean(root)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	if singleBundle(root, entries) {
		return nil, errNotRoot
	}
	var diags []wfe.Diagnostic
	for _, value := range distinct(values) {
		valueDiags, err := checkChanged(root, value)
		if err != nil {
			return nil, err
		}
		diags = append(diags, valueDiags...)
	}
	sortDiagnostics(diags)
	return diags, nil
}

// checkChanged checks what one value of the --changed flag names.
//
// The value is a path under the root. Two segments name a bundle; one
// segment names a file or a namespace directory; three or more name a
// bundle by their first two segments, the rest is ignored.
//
// A path that is not there is not a problem: the bundle was deleted,
// and there is nothing left to validate.
func checkChanged(root, value string) ([]wfe.Diagnostic, error) {
	segments := splitSegments(value)
	if len(segments) == 0 {
		return nil, nil
	}
	if len(segments) > 2 {
		// The rest of the path is ignored: a value names a bundle, not
		// a file of the bundle.
		segments = segments[:2]
	}
	path := filepath.Join(root, segments[0])
	if len(segments) == 2 {
		path = filepath.Join(path, segments[1])
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if len(segments) == 1 {
		if !info.IsDir() {
			// A file directly under the root: a workflow lives in a
			// bundle directory.
			return []wfe.Diagnostic{problem(path, 0, 0,
				"a workflow lives in a bundle directory: %s", bundleLayout())}, nil
		}
		// A namespace directory: the shape of its name only.
		return checkNS(path, segments[0]), nil
	}
	if !info.IsDir() {
		// A file under a namespace directory: the namespace directory
		// holds bundle directories only.
		return []wfe.Diagnostic{problem(path, 0, 0,
			"a namespace directory holds bundle directories only: %s", bundleDirLayout())}, nil
	}
	// A bundle: the shape of its address, then the rules of the bundle
	// itself.
	b := bundle{ns: segments[0], name: segments[1], dir: path}
	var diags []wfe.Diagnostic
	diags = append(diags, checkNames([]bundle{b})...)
	// The bundle uses the .yml extension only.
	extensions, err := checkExtensions(path)
	if err != nil {
		return nil, err
	}
	diags = append(diags, extensions...)
	bundleDiags, err := checkBundle(b)
	if err != nil {
		return nil, err
	}
	diags = append(diags, bundleDiags...)
	return diags, nil
}

// splitSegments returns the segments of a path.
//
// A path with a trailing slash, or with an empty segment, names the
// same thing as the path without them.
func splitSegments(value string) []string {
	var out []string
	for _, s := range strings.Split(value, "/") {
		if s == "" || s == "." {
			continue
		}
		out = append(out, s)
	}
	return out
}

// distinct returns the values without the ones that came before.
//
// A bundle named twice is checked once: a problem is reported once.
func distinct(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
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
