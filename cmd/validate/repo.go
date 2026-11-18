package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/cnlangzi/nightme/pkg/wfe"
	"gopkg.in/yaml.v3"
)

const (
	// workflowFile is the name of the file that holds a workflow.
	workflowFile = "workflow.yml"
	// actionsDir is the directory of a bundle that holds its bundled
	// actions.
	actionsDir = "actions"
	// scriptExt is the extension of a bundled action.
	scriptExt = ".sh"
	// yamlExt is the extension the repository does not use.
	yamlExt = ".yaml"
)

// The shape of the two halves of an address.
//
// The namespace is a GitHub account name: letters, digits and dashes,
// starting with a letter or a digit. The name is lowercase
// kebab-case.
var (
	nsPattern   = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)
	namePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

// builtinActions are the actions nightme registers itself.
//
// A `use:` step may name one of them. Every other name must be a
// bundled action of the same bundle.
//
// The list is the one of nightme's runtime: internal/me/action.go
// registers them (registerBuiltinActions). It cannot be imported from
// here — the package is internal — so it is kept in step by hand.
var builtinActions = map[string]bool{
	"notify":            true,
	"email":             true,
	"github_pr_comment": true,
	"github_issue":      true,
}

// bundle is one workflow and everything that belongs to it: the
// directory workflows/<ns>/<name>/.
type bundle struct {
	// ns and name are the two halves of the address. They are empty
	// when the validator was pointed at a single bundle: a single
	// bundle carries no address of its own.
	ns   string
	name string
	// dir is the bundle directory.
	dir string
}

// identity is the name the workflow carries once it is installed next
// to other workflows: <ns>-<name>.
func (b bundle) identity() string {
	if b.ns == "" || b.name == "" {
		return ""
	}
	return b.ns + "-" + b.name
}

// clean returns the path in its shortest form.
//
// A root of "workflows/" and a root of "workflows" name the same
// directory, and the README that sits next to it is the same README.
func clean(path string) string {
	return filepath.Clean(path)
}

// discover finds the bundles under root.
//
// A bundle is a directory that holds a workflow.yml. The address of a
// bundle is its path: workflows/<ns>/<name>/. A directory that sits
// directly under the root and holds no workflow.yml is still a
// bundle — that is the "the bundle holds no workflow.yml" problem,
// and the checks that follow report it.
//
// A root that holds a workflow.yml and no namespace directory is a
// single bundle: the validator then checks that one workflow and its
// attachments, and leaves the rules that need the whole tree alone.
// A root that holds both a workflow.yml and a namespace directory is
// the workflows directory — the workflow.yml is a stray file, and it
// is reported as one.
func discover(root string) ([]bundle, []wfe.Diagnostic, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil, err
	}
	if singleBundle(root, entries) {
		return []bundle{{dir: root}}, nil, nil
	}
	var bundles []bundle
	var diags []wfe.Diagnostic
	for _, nsEntry := range entries {
		nsPath := filepath.Join(root, nsEntry.Name())
		if !nsEntry.IsDir() {
			diags = append(diags, problem(nsPath, 0, 0,
				"a workflow lives in a bundle directory: %s", bundleLayout()))
			continue
		}
		nsEntries, err := os.ReadDir(nsPath)
		if err != nil {
			return nil, nil, err
		}
		for _, nameEntry := range nsEntries {
			bundlePath := filepath.Join(nsPath, nameEntry.Name())
			if !nameEntry.IsDir() {
				diags = append(diags, problem(bundlePath, 0, 0,
					"a namespace directory holds bundle directories only: %s", bundleDirLayout()))
				continue
			}
			bundles = append(bundles, bundle{
				ns:   nsEntry.Name(),
				name: nameEntry.Name(),
				dir:  bundlePath,
			})
		}
	}
	return bundles, diags, nil
}

// singleBundle reports whether the root holds a single bundle rather
// than the workflows directory.
//
// A root that holds a workflow file and no namespace directory is a
// single bundle: the validator then checks that one workflow and its
// attachments, and leaves the rules that need the whole tree alone.
func singleBundle(root string, entries []os.DirEntry) bool {
	return holdsWorkflowFile(entries) && !holdsNamespaceDir(root, entries)
}

// holdsWorkflowFile reports whether the entries hold a workflow file.
func holdsWorkflowFile(entries []os.DirEntry) bool {
	for _, e := range entries {
		if !e.IsDir() && e.Name() == workflowFile {
			return true
		}
	}
	return false
}

// holdsNamespaceDir reports whether the entries hold a namespace
// directory: a directory that holds a directory of its own, which is
// where a bundle lives.
//
// A directory that cannot be read counts: the walk that follows
// reports it.
func holdsNamespaceDir(root string, entries []os.DirEntry) bool {
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub, err := os.ReadDir(filepath.Join(root, e.Name()))
		if err != nil {
			return true
		}
		for _, s := range sub {
			if s.IsDir() {
				return true
			}
		}
	}
	return false
}

// bundleLayout is the path of the workflow file of a bundle, as the
// rules write it.
func bundleLayout() string {
	return "workflows/<ns>/<name>/" + workflowFile
}

// bundleDirLayout is the path of a bundle, as the rules write it.
func bundleDirLayout() string {
	return "workflows/<ns>/<name>/"
}

// checkExtensions reports every file that is not a .yml file.
//
// The repository uses the .yml extension only: .yaml is a second
// spelling of the same thing, and the two spellings drift apart.
//
// The error it returns is not a problem of a workflow: it says a path
// could not be read, and the caller turns it into the exit code 9.
func checkExtensions(root string) ([]wfe.Diagnostic, error) {
	var diags []wfe.Diagnostic
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(d.Name()) != yamlExt {
			return nil
		}
		diags = append(diags, problem(path, 0, 0,
			"the repository uses the .yml extension; %s is not allowed", yamlExt))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return diags, nil
}

// checkNames reports the names that are not shaped right.
//
// The namespace is the author's own choice — the reviewer decides
// whether it belongs to them — but it has to look like a GitHub
// account name. The name has to look like a name.
func checkNames(bundles []bundle) []wfe.Diagnostic {
	var diags []wfe.Diagnostic
	for _, b := range bundles {
		if b.ns != "" {
			diags = append(diags, checkNS(filepath.Dir(b.dir), b.ns)...)
		}
		if b.name != "" && !namePattern.MatchString(b.name) {
			diags = append(diags, problem(b.dir, 0, 0,
				"the name %q is not lowercase kebab-case", b.name))
		}
	}
	return diags
}

// checkNS reports the shape of one namespace.
//
// The directory is the namespace directory the problem stands in.
func checkNS(dir, ns string) []wfe.Diagnostic {
	if nsPattern.MatchString(ns) {
		return nil
	}
	return []wfe.Diagnostic{problem(dir, 0, 0,
		"the namespace %q is not a GitHub account name: letters, digits and dashes", ns)}
}

// checkIdentities reports the bundles that share an identity.
//
// The identity is the name a workflow carries once it is installed
// next to other workflows: <ns>-<name>. Two workflows that share one
// cannot both be installed, so both are reported.
func checkIdentities(bundles []bundle) []wfe.Diagnostic {
	groups := map[string][]bundle{}
	for _, b := range bundles {
		id := b.identity()
		if id == "" {
			continue
		}
		groups[id] = append(groups[id], b)
	}
	var diags []wfe.Diagnostic
	for id, group := range groups {
		if len(group) < 2 {
			continue
		}
		for _, b := range group {
			diags = append(diags, problem(b.dir, 0, 0,
				"the identity %q is used by more than one workflow: %s", id, joinDirs(group)))
		}
	}
	return diags
}

// checkActionNames reports the action names that more than one bundle
// ships.
//
// A `use:` step names an action, and the name says nothing about the
// bundle it comes from: two bundles that ship the same name would
// leave the reader guessing which one a step runs.
func checkActionNames(bundles []bundle) []wfe.Diagnostic {
	groups := map[string][]string{}
	for _, b := range bundles {
		entries, err := os.ReadDir(filepath.Join(b.dir, actionsDir))
		if err != nil {
			continue
		}
		for name, path := range bundledActions(b.dir, entries) {
			groups[name] = append(groups[name], path)
		}
	}
	var diags []wfe.Diagnostic
	for name, paths := range groups {
		if len(paths) < 2 {
			continue
		}
		for _, path := range paths {
			diags = append(diags, problem(path, 0, 0,
				"the action %q is shipped by more than one bundle: %s", name, strings.Join(paths, ", ")))
		}
	}
	return diags
}

// checkBundle checks the conventions of one bundle: the workflow file
// it holds and the actions it ships.
//
// The error it returns is not a problem of a workflow: it says a path
// could not be read, and the caller turns it into the exit code 9.
func checkBundle(b bundle) ([]wfe.Diagnostic, error) {
	var diags []wfe.Diagnostic
	entries, err := os.ReadDir(b.dir)
	if err != nil {
		return nil, err
	}
	// The bundle holds exactly one workflow file, and it is named
	// workflow.yml.
	hasWorkflow := false
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yml" {
			continue
		}
		if e.Name() == workflowFile {
			hasWorkflow = true
			continue
		}
		diags = append(diags, problem(filepath.Join(b.dir, e.Name()), 0, 0,
			"the workflow file of a bundle is named %s", workflowFile))
	}
	if !hasWorkflow {
		diags = append(diags, problem(b.dir, 0, 0,
			"the bundle holds no %s", workflowFile))
	}
	// The schema is nightme's own: the rules live in one place, so the
	// validator cannot drift from the runtime.
	workflowPath := filepath.Join(b.dir, workflowFile)
	if hasWorkflow {
		diags = append(diags, wfe.ValidateFile(workflowPath)...)
	}
	// The bundled actions: every file of the actions directory is a
	// script, and every script is executable.
	actionEntries, err := os.ReadDir(filepath.Join(b.dir, actionsDir))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	actions := bundledActions(b.dir, actionEntries)
	for _, e := range actionEntries {
		path := filepath.Join(b.dir, actionsDir, e.Name())
		if e.IsDir() || filepath.Ext(e.Name()) != scriptExt {
			diags = append(diags, problem(path, 0, 0,
				"a bundled action is a %s script", scriptExt))
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		if info.Mode()&0o111 == 0 {
			diags = append(diags, problem(path, 0, 0,
				"the bundled action %q is not executable", strings.TrimSuffix(e.Name(), scriptExt)))
		}
	}
	// The wiring: every `use:` names a built-in action or a bundled
	// action of this bundle, and every bundled action is used. A
	// bundle without a workflow file has no steps to wire.
	if hasWorkflow {
		refs, complete, err := usedActions(workflowPath)
		if err != nil {
			return nil, err
		}
		used := map[string]bool{}
		for _, ref := range refs {
			used[ref.name] = true
			if builtinActions[ref.name] {
				continue
			}
			if _, ok := actions[ref.name]; !ok {
				diags = append(diags, problem(workflowPath, ref.line, ref.column,
					"the step uses the action %q, which is neither a built-in action nor a bundled action of this workflow",
					ref.name))
			}
		}
		// A bundled action is used by no step only when the steps
		// that could use it are known.
		if complete {
			for name, path := range actions {
				if used[name] {
					continue
				}
				diags = append(diags, problem(path, 0, 0,
					"the bundled action %q is used by no step", name))
			}
		}
	}
	return diags, nil
}

// bundledActions returns the actions a bundle ships: the name of
// every script of its actions directory and the file that provides
// it.
//
// The name of an action is the name of its file without the
// extension: nightme registers a script under that name. A file that
// is not a script provides no action — it is reported as the wrong
// kind of file.
func bundledActions(dir string, entries []os.DirEntry) map[string]string {
	out := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != scriptExt {
			continue
		}
		name := strings.TrimSuffix(e.Name(), scriptExt)
		if name == "" {
			continue
		}
		out[name] = filepath.Join(dir, actionsDir, e.Name())
	}
	return out
}

// usedActions returns the actions the steps of a workflow file use.
//
// The names come from the schema: the file is decoded into a
// wfe.Workflow and bound with the arguments it declares, so what the
// runtime would run is what is checked. The places come from the tree
// of the file, so every name can be pointed at.
//
// The second value says whether the set of used actions is known. It
// is not when the file does not parse, when its steps cannot be read,
// or when a step names its action with an expression the run has not
// resolved yet: the wiring is then unknown, and nothing is said
// about it.
func usedActions(path string) ([]useRef, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		// A file that does not parse is reported by the schema
		// check; what it uses is not worth guessing.
		return nil, false, nil
	}
	root := documentRoot(&doc)
	if root == nil {
		return nil, false, nil
	}
	places, readable := usePlaces(root)
	if !readable {
		return nil, false, nil
	}
	names := map[string]bool{}
	var wf wfe.Workflow
	// A file that does not decode is reported by the schema check.
	// What it does use is still worth knowing.
	_ = root.Decode(&wf)
	// The runtime binds the arguments of a workflow before it runs
	// it: an action named by an argument is an ordinary action.
	bound, err := wfe.Bind(&wf, nil)
	if err != nil {
		bound = &wf
	}
	for _, job := range bound.Jobs {
		for _, step := range job.Steps {
			if step.Use != "" {
				names[step.Use] = true
			}
		}
	}
	refs := make([]useRef, 0, len(names))
	complete := true
	for name := range names {
		if isExpression(name) {
			// The action is chosen at runtime: the wiring cannot
			// be judged here.
			complete = false
			continue
		}
		ref := useRef{name: name}
		if p, ok := places[name]; ok {
			ref.line, ref.column = p.line, p.column
		}
		refs = append(refs, ref)
	}
	sortUseRefs(refs)
	return refs, complete, nil
}

// isExpression reports whether a text holds an expression the run has
// not resolved yet.
func isExpression(text string) bool {
	return strings.Contains(text, "${{")
}

// useRef is one `use:` step: the action it names and the place it
// stands.
type useRef struct {
	name   string
	line   int
	column int
}

// documentRoot returns the root of a parsed document.
func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc == nil {
		return nil
	}
	if doc.Kind == yaml.DocumentNode {
		if len(doc.Content) == 0 {
			return nil
		}
		return doc.Content[0]
	}
	return doc
}

// usePlaces returns where each `use:` of a workflow file stands, and
// whether the steps of the file could be read at all.
//
// The walk follows the shape of the schema: the jobs hold the steps,
// and the steps hold the `use:`. A file whose shape is broken is
// reported by the schema check; what it uses is not worth guessing.
func usePlaces(root *yaml.Node) (map[string]useRef, bool) {
	out := map[string]useRef{}
	if root.Kind != yaml.MappingNode {
		return out, false
	}
	for _, jobs := range mappingValues(root, "jobs") {
		if jobs.Kind != yaml.MappingNode {
			return out, false
		}
		for _, job := range mappingItems(jobs) {
			if job.Kind != yaml.MappingNode {
				return out, false
			}
			for _, steps := range mappingValues(job, "steps") {
				if steps.Kind != yaml.SequenceNode {
					return out, false
				}
				for _, step := range steps.Content {
					if step.Kind != yaml.MappingNode {
						return out, false
					}
					for i := 0; i+1 < len(step.Content); i += 2 {
						key, val := step.Content[i], step.Content[i+1]
						if key.Value != "use" || val.Kind != yaml.ScalarNode {
							continue
						}
						if _, ok := out[val.Value]; ok {
							continue
						}
						out[val.Value] = useRef{name: val.Value, line: key.Line, column: key.Column}
					}
				}
			}
		}
	}
	return out, true
}

// mappingItems returns the values of every key of a mapping.
func mappingItems(node *yaml.Node) []*yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	var out []*yaml.Node
	for i := 1; i < len(node.Content); i += 2 {
		out = append(out, node.Content[i])
	}
	return out
}

// mappingValues returns the values the key holds in a mapping.
func mappingValues(node *yaml.Node, key string) []*yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	var out []*yaml.Node
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			out = append(out, node.Content[i+1])
		}
	}
	return out
}

// sortUseRefs orders the references by name, so the problems they
// produce come in a stable order.
func sortUseRefs(refs []useRef) {
	sort.Slice(refs, func(i, j int) bool { return refs[i].name < refs[j].name })
}

// joinDirs returns the directories of a group of bundles.
func joinDirs(group []bundle) string {
	dirs := make([]string, 0, len(group))
	for _, b := range group {
		dirs = append(dirs, b.dir)
	}
	return strings.Join(dirs, ", ")
}
