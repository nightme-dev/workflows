package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cnlangzi/nightme/pkg/wfe"
)

// workflowFile is the name of the workflow file of a bundle. It is
// the only file of a bundle the generator reads a workflow from.
const workflowFile = "workflow.yml"

// readmeFile is the file of a bundle whose first paragraph is the
// summary of the workflow.
const readmeFile = "README.md"

// actionsDir is the directory of a bundle whose scripts the workflow
// ships.
const actionsDir = "actions"

// scriptExt is the extension of the scripts of the actions directory.
const scriptExt = ".sh"

// reservedPaths are the root paths the site uses for itself.
//
// A namespace is a single path segment, so a namespace that takes
// one of these names would be the same URL as the thing that holds
// it: the namespace page of "changes" and the changes page would be
// the same page. A workflow is two segments deep, so it never
// collides.
var reservedPaths = []string{"assets", "changes", "pagefind"}

// reservedNames are the names a namespace may not take because a page
// of the site already has their file name.
//
// The page of a namespace is written to "<namespace>.md", and the
// home page is written to "index.md": a namespace named "index"
// would write over the home page.
var reservedNames = []string{"index"}

// bundle is one workflow directory of the repository.
type bundle struct {
	// namespace is the GitHub account name the workflow is published
	// under.
	namespace string
	// name is the short name of the workflow.
	name string
	// address is "<namespace>/<name>": the path of the bundle in
	// the repository.
	address string
	// identity is "<namespace>-<name>": the name the workflow
	// carries once it is installed.
	identity string
	// path is the path of the bundle relative to the root of the
	// repository.
	path string
	// workflow is the parsed workflow file.
	workflow *wfe.Workflow
	// summary is the first paragraph of the README of the bundle.
	summary string
	// actions are the scripts the bundle ships.
	actions []actionRecord
}

// siteData is everything the site is built from.
type siteData struct {
	// Records are the workflows, ordered by address.
	Records []workflowRecord
	// Changes are the commits that touched the workflows, the newest
	// first.
	Changes []changeRecord
	// Namespaces are the namespaces of the workflows, in order.
	Namespaces []string
}

// workflowRecord is one workflow as the templates read it.
//
// The fields are the ones the layouts of the site use: a layout reads
// the record and renders it, it does not read the workflow file.
type workflowRecord struct {
	Address    string         `json:"address"`
	Identity   string         `json:"identity"`
	Namespace  string         `json:"namespace"`
	Name       string         `json:"name"`
	Title      string         `json:"title"`
	Summary    string         `json:"summary"`
	Args       []argRecord    `json:"args"`
	Instruct   bool           `json:"instruct"`
	Workspaces []string       `json:"workspaces"`
	On         wfe.Trigger    `json:"on"`
	Jobs       []jobRecord    `json:"jobs"`
	Actions    []actionRecord `json:"actions"`
	Source     string         `json:"source"`
	LastCommit commitRecord   `json:"last_commit"`
}

// argRecord is one argument of a workflow.
//
// An argument with a default is optional, an argument without one is
// required: the run cannot start without a value for it.
type argRecord struct {
	Name     string `json:"name"`
	Default  string `json:"default"`
	Required bool   `json:"required"`
}

// jobRecord is one job of a workflow.
//
// The sleep is the gap between two rounds of the job: a job that
// names no gap has none, and its next round starts at once.
type jobRecord struct {
	Name       string       `json:"name"`
	Needs      []string     `json:"needs"`
	If         string       `json:"if"`
	Iterations int          `json:"iterations"`
	Sleep      string       `json:"sleep"`
	Steps      []stepRecord `json:"steps"`
}

// stepRecord is one step of a job.
//
// The kind is what the step does: it says which of the four things a
// step can be — a message, a shell command, an action, a wait — it
// is. A step that is none of them has no kind.
type stepRecord struct {
	ID      string            `json:"id"`
	Kind    string            `json:"kind"`
	If      string            `json:"if"`
	Env     map[string]string `json:"env"`
	Note    string            `json:"note"`
	Final   bool              `json:"final"`
	Message string            `json:"message"`
	Shell   string            `json:"shell"`
	Use     string            `json:"use"`
	With    map[string]any    `json:"with"`
	Wait    string            `json:"wait"`
}

// actionRecord is one script a bundle ships.
type actionRecord struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// commitRecord is one commit of the history.
type commitRecord struct {
	SHA     string `json:"sha"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
	URL     string `json:"url"`
}

// changeRecord is one commit that touched the workflows.
type changeRecord struct {
	SHA      string   `json:"sha"`
	Date     string   `json:"date"`
	Subject  string   `json:"subject"`
	URL      string   `json:"url"`
	Added    []string `json:"added"`
	Modified []string `json:"modified"`
	Removed  []string `json:"removed"`
}

// build reads the repository and returns the facts the site is made
// of.
//
// The templates are read first: a site without them is not a site,
// and the reader wants to know that before the workflows are read.
func build(templates, workflowsRoot string, limit int) (*siteData, error) {
	if err := checkDir(templates); err != nil {
		return nil, err
	}
	if err := checkDir(workflowsRoot); err != nil {
		return nil, err
	}
	// The history is opened first: it knows where the workflows live
	// in the repository, and the links of the site are built from it.
	hist, err := openHistory(workflowsRoot)
	if err != nil {
		return nil, err
	}
	bundles, err := readBundles(workflowsRoot, hist.workflows)
	if err != nil {
		return nil, err
	}
	if err := checkNamespaces(bundles); err != nil {
		return nil, err
	}
	return buildSite(bundles, hist, limit)
}

// buildSite turns the bundles of the repository and the history into
// the facts the site is built from.
func buildSite(bundles []bundle, hist history, limit int) (*siteData, error) {
	records := make([]workflowRecord, 0, len(bundles))
	namespaces := map[string]bool{}
	for _, b := range bundles {
		records = append(records, b.record(hist))
		namespaces[b.namespace] = true
	}
	changes, err := hist.changes(limit)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(namespaces))
	for ns := range namespaces {
		names = append(names, ns)
	}
	sort.Strings(names)
	return &siteData{Records: records, Changes: changes, Namespaces: names}, nil
}

// record turns a bundle into the record the templates read.
func (b bundle) record(hist history) workflowRecord {
	rec := workflowRecord{
		Address:    b.address,
		Identity:   b.identity,
		Namespace:  b.namespace,
		Name:       b.name,
		Title:      b.workflow.Title,
		Summary:    b.summary,
		Args:       argsOf(b.workflow),
		Instruct:   b.workflow.Instruct,
		Workspaces: workspacesOf(b.workflow),
		On:         b.workflow.On,
		Jobs:       jobsOf(b.workflow),
		Actions:    b.actions,
		Source:     blobURL(b.path + "/" + workflowFile),
		LastCommit: commitRecord{},
	}
	if commit, ok := hist.lastCommit(b.address); ok {
		rec.LastCommit = commit
	}
	return rec
}

// argsOf returns the arguments of a workflow.
//
// An argument with no default is required: the run cannot start
// without a value for it. An argument with a default is optional.
func argsOf(wf *wfe.Workflow) []argRecord {
	out := make([]argRecord, 0, len(wf.Args))
	for _, arg := range wf.Args {
		rec := argRecord{Name: arg.Name}
		if arg.Default != nil {
			rec.Default = *arg.Default
		} else {
			rec.Required = true
		}
		out = append(out, rec)
	}
	return out
}

// workspacesOf returns the directories a workflow runs in.
func workspacesOf(wf *wfe.Workflow) []string {
	out := make([]string, 0, len(wf.Workspaces))
	out = append(out, wf.Workspaces...)
	return out
}

// jobsOf returns the jobs of a workflow, ordered by name.
//
// The order is not the order of the file: a map has no order of its
// own, and the site has to show the same jobs in the same order every
// time.
func jobsOf(wf *wfe.Workflow) []jobRecord {
	names := make([]string, 0, len(wf.Jobs))
	for name := range wf.Jobs {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]jobRecord, 0, len(names))
	for _, name := range names {
		job := wf.Jobs[name]
		needs := make([]string, 0, len(job.Needs))
		needs = append(needs, job.Needs...)
		out = append(out, jobRecord{
			Name:       name,
			Needs:      needs,
			If:         job.If,
			Iterations: job.Rounds(),
			Sleep:      job.Sleep,
			Steps:      stepsOf(job.Steps),
		})
	}
	return out
}

// stepsOf returns the steps of a job.
func stepsOf(steps []wfe.Step) []stepRecord {
	out := make([]stepRecord, 0, len(steps))
	for _, step := range steps {
		rec := stepRecord{
			ID:      step.ID,
			Kind:    kindOf(step),
			If:      step.If,
			Note:    step.Note,
			Final:   step.Final,
			Message: step.Message,
			Shell:   step.Shell,
			Use:     step.Use,
			Wait:    step.Wait,
			Env:     map[string]string{},
			With:    map[string]any{},
		}
		for name, value := range step.Env {
			rec.Env[name] = value
		}
		for name, value := range step.With {
			rec.With[name] = value
		}
		out = append(out, rec)
	}
	return out
}

// kindOf returns what a step does.
//
// The four kinds are the four things a step can be; a step that is
// none of them has no kind.
func kindOf(step wfe.Step) string {
	switch step.Kind() {
	case wfe.StepKindMessage:
		return "message"
	case wfe.StepKindShell:
		return "shell"
	case wfe.StepKindUse:
		return "use"
	case wfe.StepKindWait:
		return "wait"
	}
	return ""
}

// readBundles reads the bundles of the workflows directory.
//
// A directory that holds no workflow file is not a bundle: the
// validator is the one that reports it, the site has nothing to say
// about it.
func readBundles(root, workflows string) ([]bundle, error) {
	namespaces, err := readDir(root)
	if err != nil {
		return nil, err
	}
	var out []bundle
	for _, ns := range namespaces {
		names, err := readDir(filepath.Join(root, ns))
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			b, ok, err := readBundle(root, workflows, ns, name)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].address < out[j].address })
	return out, nil
}

// readBundle reads one bundle of the workflows directory.
//
// The second value says whether the directory holds a workflow file.
func readBundle(root, workflows, namespace, name string) (bundle, bool, error) {
	dir := filepath.Join(root, namespace, name)
	file := filepath.Join(dir, workflowFile)
	raw, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return bundle{}, false, nil
		}
		return bundle{}, false, &unreadableError{reason: fmt.Sprintf("%s: %v", file, err)}
	}
	wf, err := wfe.Parse(raw)
	if err != nil {
		// A workflow that does not validate cannot be described:
		// the site would say something the runtime does not do.
		return bundle{}, false, &invalidError{reason: fmt.Sprintf("%s: %v", file, err)}
	}
	return bundle{
		namespace: namespace,
		name:      name,
		address:   namespace + "/" + name,
		identity:  namespace + "-" + name,
		path:      filepath.ToSlash(filepath.Join(workflows, namespace, name)),
		workflow:  wf,
		summary:   readSummary(dir),
		actions:   readActions(dir),
	}, true, nil
}

// readSummary returns the first paragraph of the README of a bundle.
//
// The first paragraph is the first block of text that is not a
// heading: it is what the site shows as the summary of the
// workflow. A bundle without a README has no summary.
func readSummary(dir string) string {
	raw, err := os.ReadFile(filepath.Join(dir, readmeFile))
	if err != nil {
		return ""
	}
	return firstParagraph(string(raw))
}

// firstParagraph returns the first paragraph of a markdown text.
//
// A heading is not a paragraph, and the text before a paragraph —
// the title of the file — is not one either.
func firstParagraph(text string) string {
	for _, block := range strings.Split(text, "\n\n") {
		para := strings.TrimSpace(block)
		if para == "" {
			continue
		}
		if strings.HasPrefix(para, "#") {
			continue
		}
		return para
	}
	return ""
}

// readActions returns the scripts a bundle ships.
func readActions(dir string) []actionRecord {
	out := []actionRecord{}
	entries, err := os.ReadDir(filepath.Join(dir, actionsDir))
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), scriptExt) {
			continue
		}
		out = append(out, actionRecord{
			Name: strings.TrimSuffix(entry.Name(), scriptExt),
			Path: actionsDir + "/" + entry.Name(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// checkNamespaces refuses the namespaces that take a root path of the
// site.
//
// The namespace page of such a namespace would be the page of the
// thing that holds it, and there is no way to tell them apart.
func checkNamespaces(bundles []bundle) error {
	for _, b := range bundles {
		for _, reserved := range reservedPaths {
			if b.namespace == reserved {
				return &invalidError{reason: fmt.Sprintf(
					"the namespace %q takes the root path %q of the site; a namespace may not be named %s",
					b.namespace, reserved, strings.Join(reservedPaths, ", "))}
			}
		}
		for _, reserved := range reservedNames {
			if b.namespace == reserved {
				return &invalidError{reason: fmt.Sprintf(
					"the namespace %q takes the page %q of the site; a namespace may not be named %s",
					b.namespace, reserved+".md", strings.Join(reservedNames, ", "))}
			}
		}
	}
	return nil
}

// checkDir says whether a directory can be read.
func checkDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return &unreadableError{reason: fmt.Sprintf("%s: %v", path, err)}
	}
	if !info.IsDir() {
		return &unreadableError{reason: fmt.Sprintf("%s: not a directory", path)}
	}
	return nil
}

// readDir returns the names of the directories of a directory.
//
// The hidden directories are left out: they are not part of the
// repository.
func readDir(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, &unreadableError{reason: fmt.Sprintf("%s: %v", path, err)}
	}
	var out []string
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		out = append(out, entry.Name())
	}
	sort.Strings(out)
	return out, nil
}
