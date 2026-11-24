package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A workflow that parses: a title, one argument and one job.
const siteWorkflow = `title: a workflow
args:
  - name: issue
    default: "*"
jobs:
  ship:
    iterations: 0
    steps:
      - id: fix
        message: hello
`

// tree is a repository under test: a workflows directory and the
// templates of the site.
type tree struct {
	t   *testing.T
	dir string
}

// newTree returns a repository that holds nothing yet.
func newTree(t *testing.T) *tree {
	t.Helper()
	return &tree{t: t, dir: t.TempDir()}
}

// write writes a file of the tree and makes the directories it needs.
func (tr *tree) write(path, content string) string {
	tr.t.Helper()
	full := filepath.Join(tr.dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		tr.t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		tr.t.Fatalf("write %s: %v", full, err)
	}
	return full
}

// bundle writes a bundle with a workflow file.
func (tr *tree) bundle(ns, name, workflow string) {
	tr.t.Helper()
	tr.write(filepath.Join("workflows", ns, name, workflowFile), workflow)
}

// readme writes the README of a bundle.
func (tr *tree) readme(ns, name, content string) {
	tr.t.Helper()
	tr.write(filepath.Join("workflows", ns, name, readmeFile), content)
}

// script writes an action of a bundle.
func (tr *tree) script(ns, name, script string) {
	tr.t.Helper()
	tr.write(filepath.Join("workflows", ns, name, actionsDir, script), "#!/bin/sh\nexit 0\n")
}

// templates writes a directory of templates.
func (tr *tree) templates() string {
	tr.t.Helper()
	tr.write(filepath.Join("templates", "_config.yml"), "title: a site\nbaseurl: \"/workflows\"\n")
	tr.write(filepath.Join("templates", "_layouts", "default.html"), "<html>{{ content }}</html>\n")
	tr.write(filepath.Join("templates", "_includes", "nav.html"), "<nav></nav>\n")
	tr.write(filepath.Join("templates", "assets", "style.css"), "body {}\n")
	return filepath.Join(tr.dir, "templates")
}

// workflows returns the workflows directory of the tree.
func (tr *tree) workflows() string {
	return filepath.Join(tr.dir, "workflows")
}

// commit makes a commit of the tree and returns its short hash.
//
// The tree is a repository of its own, so the history the generator
// reads is the history of the test.
func (tr *tree) commit(message string) string {
	tr.t.Helper()
	tr.git("add", "-A")
	tr.git("-c", "user.name=nightme", "-c", "user.email=nightme@example.com",
		"commit", "-m", message)
	out := strings.TrimSpace(tr.git("rev-parse", "--short", "HEAD"))
	return out
}

// git runs a git command in the tree.
func (tr *tree) git(args ...string) string {
	tr.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", tr.dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		tr.t.Fatalf("git %s: %v:\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// init makes the tree a repository. The commits are the business of
// the test.
func (tr *tree) init() {
	tr.t.Helper()
	tr.git("init", "--quiet")
}

// fakeHistory is a history that says what the test wants it to say.
type fakeHistory struct {
	// commits are the last commits of the bundles, by path.
	commits map[string]commitRecord
	// touched are the commits that touched the workflows.
	touched []changeRecord
	// last is the date of the newest commit that touched the
	// workflows.
	last string
}

// lastCommit returns the commit of the bundle at the path.
func (h fakeHistory) lastCommit(relPath string) (commitRecord, bool) {
	commit, ok := h.commits[relPath]
	return commit, ok
}

// changes returns the commits that touched the workflows.
func (h fakeHistory) changes(limit int) ([]changeRecord, error) {
	if limit < len(h.touched) {
		return h.touched[:limit], nil
	}
	return h.touched, nil
}

// lastChange returns the date of the newest commit that touched the
// workflows.
func (h fakeHistory) lastChange() (string, bool) {
	return h.last, h.last != ""
}

// noHistory is a history that knows nothing.
var noHistory = fakeHistory{}

// recordOf returns the record of an address.
func recordOf(t *testing.T, data *siteData, address string) workflowRecord {
	t.Helper()
	for _, record := range data.Records {
		if record.Address == address {
			return record
		}
	}
	t.Fatalf("the site holds no record of %q", address)
	return workflowRecord{}
}

// readIndex reads the index of the API of a site source.
func readIndex(t *testing.T, dir string) apiIndex {
	t.Helper()
	raw := readFile(t, filepath.Join(dir, "api", "index.json"))
	var index apiIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatalf("the index of the API is not JSON: %v", err)
	}
	return index
}

// readJSON reads a JSON file as it is written.
func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	raw := readFile(t, path)
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("%s is not JSON: %v", path, err)
	}
	return value
}

// readFile reads a file of the tree.
func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}

// siteOf builds the site of the bundles with the history.
func siteOf(t *testing.T, bundles []bundle, hist history) *siteData {
	t.Helper()
	data, err := buildSite(bundles, hist, defaultChangesLimit, defaultSiteURL)
	if err != nil {
		t.Fatalf("buildSite: %v", err)
	}
	return data
}

// TestArgsAreMapped checks what an argument becomes.
//
// An argument with a default is optional, an argument without one is
// required: the run cannot start without a value for it.
func TestArgsAreMapped(t *testing.T) {
	cases := []struct {
		name     string
		workflow string
		want     []argRecord
	}{
		{
			name: "an argument with a default is optional",
			workflow: `title: a workflow
args:
  - name: issue
    default: "*"
jobs:
  ship:
    steps:
      - id: fix
        message: hello
`,
			want: []argRecord{{Name: "issue", Default: "*"}},
		},
		{
			name: "an argument without a default is required",
			workflow: `title: a workflow
args:
  - name: issue
jobs:
  ship:
    steps:
      - id: fix
        message: hello
`,
			want: []argRecord{{Name: "issue", Required: true}},
		},
		{
			name: "a workflow without arguments has none",
			workflow: `title: a workflow
jobs:
  ship:
    steps:
      - id: fix
        message: hello
`,
			want: []argRecord{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tr := newTree(t)
			tr.bundle("alice", "fix", c.workflow)
			bundles, err := readBundles(tr.workflows(), "workflows")
			if err != nil {
				t.Fatalf("readBundles: %v", err)
			}
			record := recordOf(t, siteOf(t, bundles, noHistory), "alice/fix")
			if len(record.Args) != len(c.want) {
				t.Fatalf("the workflow holds %d arguments, want %d: %+v", len(record.Args), len(c.want), record.Args)
			}
			for i, want := range c.want {
				if record.Args[i] != want {
					t.Errorf("the argument %d is %+v, want %+v", i, record.Args[i], want)
				}
			}
		})
	}
}

// TestTheKindOfAStep checks what a step becomes.
//
// A step is a message, a shell command, an action, a wait or a builtin
// command. A step that is none of them has no kind.
func TestTheKindOfAStep(t *testing.T) {
	cases := []struct {
		name string
		step string
		want string
		cmd  string
	}{
		{name: "a message", step: "message: hello", want: "message"},
		{name: "a shell command", step: "shell: ls", want: "shell"},
		{name: "an action", step: "use: nightme-report", want: "use"},
		{name: "a wait", step: "wait: 5m", want: "wait"},
		{
			name: "a builtin command",
			step: "cmd:\n          name: skip",
			want: "cmd",
			cmd:  "skip",
		},
		{
			// The runtime trims the name before it runs the command,
			// so the site has to trim it too.
			name: "a builtin command with spaces around its name",
			step: "cmd:\n          name: \" skip \"",
			want: "cmd",
			cmd:  "skip",
		},
		{
			// A cmd step may carry a message and name no command.
			name: "a builtin command that names no command",
			step: "cmd:\n          name: \"\"\n        message: hello",
			want: "cmd",
			cmd:  "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tr := newTree(t)
			tr.bundle("alice", "fix", "title: a workflow\njobs:\n  ship:\n    steps:\n      - id: fix\n        "+c.step+"\n")
			bundles, err := readBundles(tr.workflows(), "workflows")
			if err != nil {
				t.Fatalf("readBundles: %v", err)
			}
			record := recordOf(t, siteOf(t, bundles, noHistory), "alice/fix")
			if got := record.Jobs[0].Steps[0].Kind; got != c.want {
				t.Errorf("the step is a %q, want a %q", got, c.want)
			}
			if got := record.Jobs[0].Steps[0].Cmd; got != c.cmd {
				t.Errorf("the step runs the builtin %q, want %q", got, c.cmd)
			}
		})
	}
}

// TestTheRoundsOfAJob checks what the iterations of a job become.
//
// Zero rounds means the job runs until the run ends, a missing value
// means one round.
func TestTheRoundsOfAJob(t *testing.T) {
	cases := []struct {
		name string
		job  string
		want int
	}{
		{name: "no iterations is one round", job: "  ship:\n", want: 1},
		{name: "zero iterations is until the end", job: "  ship:\n    iterations: 0\n", want: 0},
		{name: "three iterations is three rounds", job: "  ship:\n    iterations: 3\n", want: 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tr := newTree(t)
			tr.bundle("alice", "fix", "title: a workflow\njobs:\n"+c.job+"    steps:\n      - id: fix\n        message: hello\n")
			bundles, err := readBundles(tr.workflows(), "workflows")
			if err != nil {
				t.Fatalf("readBundles: %v", err)
			}
			record := recordOf(t, siteOf(t, bundles, noHistory), "alice/fix")
			if got := record.Jobs[0].Iterations; got != c.want {
				t.Errorf("the job runs %d rounds, want %d", got, c.want)
			}
		})
	}
}

// TestTheSleepOfAJob checks what the sleep of a job becomes.
//
// The sleep is the gap between two rounds of the job. A job that
// names no gap has none: its next round starts at once.
func TestTheSleepOfAJob(t *testing.T) {
	cases := []struct {
		name string
		job  string
		want string
	}{
		{name: "no sleep is no gap", job: "  ship:\n", want: ""},
		{name: "the sleep is the gap", job: "  ship:\n    sleep: 5m\n", want: "5m"},
		{name: "a short gap is a gap", job: "  ship:\n    sleep: 30s\n", want: "30s"},
		{name: "a gap of zero is no gap", job: "  ship:\n    sleep: 0s\n", want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tr := newTree(t)
			tr.bundle("alice", "fix", "title: a workflow\njobs:\n"+c.job+"    steps:\n      - id: fix\n        message: hello\n")
			bundles, err := readBundles(tr.workflows(), "workflows")
			if err != nil {
				t.Fatalf("readBundles: %v", err)
			}
			record := recordOf(t, siteOf(t, bundles, noHistory), "alice/fix")
			if got := record.Jobs[0].Sleep; got != c.want {
				t.Errorf("the job waits %q between its rounds, want %q", got, c.want)
			}
		})
	}
}

// TestTheSummaryIsTheFirstParagraph checks what the README becomes.
//
// The first paragraph is the first block of text that is not a
// heading, and a bundle without a README has no summary.
func TestTheSummaryIsTheFirstParagraph(t *testing.T) {
	cases := []struct {
		name   string
		readme string
		want   string
	}{
		{
			name:   "the first paragraph is the summary",
			readme: "# The title\n\nThe first paragraph.\n\nThe second paragraph.\n",
			want:   "The first paragraph.",
		},
		{
			name:   "a paragraph without a title is the summary",
			readme: "The first paragraph.\n\nThe second paragraph.\n",
			want:   "The first paragraph.",
		},
		{
			name:   "a README of headings has no summary",
			readme: "# The title\n\n## Another title\n",
			want:   "",
		},
		{
			name:   "an empty README has no summary",
			readme: "",
			want:   "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tr := newTree(t)
			tr.bundle("alice", "fix", siteWorkflow)
			if c.readme != "" {
				tr.readme("alice", "fix", c.readme)
			}
			bundles, err := readBundles(tr.workflows(), "workflows")
			if err != nil {
				t.Fatalf("readBundles: %v", err)
			}
			record := recordOf(t, siteOf(t, bundles, noHistory), "alice/fix")
			if record.Summary != c.want {
				t.Errorf("the summary is %q, want %q", record.Summary, c.want)
			}
		})
	}
}

// TestTheActionsOfABundle checks what the scripts of a bundle become.
func TestTheActionsOfABundle(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", siteWorkflow)
	tr.script("alice", "fix", "nightme-alice-report.sh")
	tr.script("alice", "fix", "notes.txt")
	bundles, err := readBundles(tr.workflows(), "workflows")
	if err != nil {
		t.Fatalf("readBundles: %v", err)
	}
	record := recordOf(t, siteOf(t, bundles, noHistory), "alice/fix")
	want := []actionRecord{{Name: "nightme-alice-report", Path: "actions/nightme-alice-report.sh"}}
	if len(record.Actions) != len(want) {
		t.Fatalf("the bundle ships %d actions, want %d: %+v", len(record.Actions), len(want), record.Actions)
	}
	if record.Actions[0] != want[0] {
		t.Errorf("the action is %+v, want %+v", record.Actions[0], want[0])
	}
}

// TestABundleWithoutActionsShipsNone checks that a bundle without
// actions ships none.
//
// The list is a list and not nothing: the data is read by other
// programs too.
func TestABundleWithoutActionsShipsNone(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", siteWorkflow)
	bundles, err := readBundles(tr.workflows(), "workflows")
	if err != nil {
		t.Fatalf("readBundles: %v", err)
	}
	record := recordOf(t, siteOf(t, bundles, noHistory), "alice/fix")
	if record.Actions == nil {
		t.Error("the actions of the bundle are nothing, want an empty list")
	}
	if len(record.Actions) != 0 {
		t.Errorf("the bundle ships %d actions, want none", len(record.Actions))
	}
}

// TestTheJobsAreSortedByName checks the order of the jobs.
//
// The order of a map is not an order: the site shows the same jobs in
// the same order every time.
func TestTheJobsAreSortedByName(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", `title: a workflow
jobs:
  zulu:
    steps:
      - id: zulu
        message: hello
  alpha:
    steps:
      - id: alpha
        message: hello
  mike:
    steps:
      - id: mike
        message: hello
`)
	bundles, err := readBundles(tr.workflows(), "workflows")
	if err != nil {
		t.Fatalf("readBundles: %v", err)
	}
	record := recordOf(t, siteOf(t, bundles, noHistory), "alice/fix")
	want := []string{"alpha", "mike", "zulu"}
	if len(record.Jobs) != len(want) {
		t.Fatalf("the workflow holds %d jobs, want %d", len(record.Jobs), len(want))
	}
	for i, name := range want {
		if record.Jobs[i].Name != name {
			t.Errorf("the job %d is %q, want %q", i, record.Jobs[i].Name, name)
		}
	}
}

// TestTheWorkflowsAreSortedByAddress checks the order of the records.
func TestTheWorkflowsAreSortedByAddress(t *testing.T) {
	tr := newTree(t)
	tr.bundle("zulu", "fix", siteWorkflow)
	tr.bundle("alice", "fix", siteWorkflow)
	tr.bundle("alice", "alpha", siteWorkflow)
	bundles, err := readBundles(tr.workflows(), "workflows")
	if err != nil {
		t.Fatalf("readBundles: %v", err)
	}
	data := siteOf(t, bundles, noHistory)
	want := []string{"alice/alpha", "alice/fix", "zulu/fix"}
	if len(data.Records) != len(want) {
		t.Fatalf("the site holds %d records, want %d", len(data.Records), len(want))
	}
	for i, address := range want {
		if data.Records[i].Address != address {
			t.Errorf("the record %d is %q, want %q", i, data.Records[i].Address, address)
		}
	}
}

// TestTheChangesAreClassified checks what a commit did to the
// workflows.
//
// The workflow file of a bundle says whether the bundle came, went or
// stayed.
func TestTheChangesAreClassified(t *testing.T) {
	changes := []fileChange{
		{status: "A", path: "workflows/alice/new/workflow.yml"},
		{status: "M", path: "workflows/alice/changed/workflow.yml"},
		{status: "D", path: "workflows/alice/gone/workflow.yml"},
		{status: "M", path: "workflows/alice/notes/README.md"},
		{status: "A", path: "workflows/README.md"},
	}
	added, modified, removed := classify(changes, "workflows")
	wantAdded := []string{"alice/new"}
	wantModified := []string{"alice/changed", "alice/notes"}
	wantRemoved := []string{"alice/gone"}
	if !equal(added, wantAdded) {
		t.Errorf("the added addresses are %v, want %v", added, wantAdded)
	}
	if !equal(modified, wantModified) {
		t.Errorf("the changed addresses are %v, want %v", modified, wantModified)
	}
	if !equal(removed, wantRemoved) {
		t.Errorf("the removed addresses are %v, want %v", removed, wantRemoved)
	}
}

// TestTheReservedNamespacesAreRefused checks the namespaces that take
// a root path of the site.
//
// The namespace page of such a namespace would be the page of the
// thing that holds it.
func TestTheReservedNamespacesAreRefused(t *testing.T) {
	for _, reserved := range reservedPaths {
		t.Run(reserved, func(t *testing.T) {
			tr := newTree(t)
			tr.bundle(reserved, "fix", siteWorkflow)
			tr.templates()
			tr.init()
			t.Chdir(tr.dir)
			code, _, errOut := runProgram(t)
			if code != exitInvalid {
				t.Errorf("the namespace %q exits %d, want %d:\n%s", reserved, code, exitInvalid, errOut)
			}
			if !strings.Contains(errOut, reserved) {
				t.Errorf("the error does not name the namespace %q:\n%s", reserved, errOut)
			}
		})
	}
}

// TestTheReservedNamesAreRefused checks the namespaces that take the
// name of a page of the site.
//
// The page of a namespace is written to "<namespace>.md", and the
// home page is written to "index.md": a namespace named "index"
// would write over the home page.
func TestTheReservedNamesAreRefused(t *testing.T) {
	for _, reserved := range reservedNames {
		t.Run(reserved, func(t *testing.T) {
			tr := newTree(t)
			tr.bundle(reserved, "fix", siteWorkflow)
			tr.templates()
			tr.init()
			tr.commit("add the workflow")
			t.Chdir(tr.dir)
			code, _, errOut := runProgram(t)
			if code != exitInvalid {
				t.Errorf("the namespace %q exits %d, want %d:\n%s", reserved, code, exitInvalid, errOut)
			}
			if !strings.Contains(errOut, reserved) {
				t.Errorf("the error does not name the namespace %q:\n%s", reserved, errOut)
			}
		})
	}
}

// TestTheFrontMatterQuotesWhatYAMLWouldReadAsSomethingElse checks the
// front matter of a page.
//
// A namespace named "no" is a namespace named "no" and not the
// boolean YAML reads in it.
func TestTheFrontMatterQuotesWhatYAMLWouldReadAsSomethingElse(t *testing.T) {
	cases := []struct {
		name string
		ns   string
		want string
	}{
		{name: "a name YAML reads as a string is left alone", ns: "alice", want: "namespace: alice\n"},
		{name: "a name YAML reads as a boolean is quoted", ns: "no", want: "namespace: \"no\"\n"},
		{name: "a name YAML reads as a number is quoted", ns: "123", want: "namespace: \"123\"\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tr := newTree(t)
			tr.templates()
			tr.bundle(c.ns, "fix", siteWorkflow)
			tr.init()
			tr.commit("add the workflow")
			t.Chdir(tr.dir)
			code, _, errOut := runProgram(t)
			if code != exitOK {
				t.Fatalf("the program exits %d, want %d:\n%s", code, exitOK, errOut)
			}
			raw, err := os.ReadFile(filepath.Join(tr.dir, "docs", c.ns+".md"))
			if err != nil {
				t.Fatalf("read the page: %v", err)
			}
			if !strings.Contains(string(raw), c.want) {
				t.Errorf("the front matter does not hold %q:\n%s", c.want, raw)
			}
		})
	}
}

// TestTheProgramWritesTheSite runs the program and looks at what it
// wrote.
func TestTheProgramWritesTheSite(t *testing.T) {
	tr := newTree(t)
	tr.templates()
	tr.bundle("alice", "fix", siteWorkflow)
	tr.readme("alice", "fix", "# The title\n\nThe first paragraph.\n")
	tr.init()
	tr.commit("add the workflow")
	t.Chdir(tr.dir)
	code, out, errOut := runProgram(t, "--templates", "templates", "--workflows", "workflows", "--out", "docs")
	if code != exitOK {
		t.Fatalf("the program exits %d, want %d:\n%s %s", code, exitOK, out, errOut)
	}
	want := []string{
		"docs/_data/workflows.json",
		"docs/_data/changes.json",
		"docs/index.md",
		"docs/changes.md",
		"docs/alice.md",
		"docs/alice/fix.md",
		"docs/_config.yml",
		"docs/_layouts/default.html",
		"docs/_includes/nav.html",
		"docs/assets/style.css",
	}
	for _, path := range want {
		if _, err := os.Stat(filepath.Join(tr.dir, path)); err != nil {
			t.Errorf("the program did not write %s: %v", path, err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(tr.dir, "docs/_data/workflows.json"))
	if err != nil {
		t.Fatalf("read the workflows: %v", err)
	}
	var records []workflowRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		t.Fatalf("the workflows are not JSON: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("the site holds %d workflows, want 1", len(records))
	}
	if records[0].Summary != "The first paragraph." {
		t.Errorf("the summary is %q, want %q", records[0].Summary, "The first paragraph.")
	}
	if records[0].Source != "https://github.com/nightme-dev/workflows/blob/main/workflows/alice/fix/workflow.yml" {
		t.Errorf("the source is %q", records[0].Source)
	}
	if records[0].LastCommit.SHA == "" {
		t.Error("the record does not say which commit changed the workflow last")
	}
}

// TestTheProgramWritesTheAPI runs the program and looks at the files
// of the API.
//
// The index lists every workflow of the repository, and every workflow
// has a file of its own that holds its whole record.
func TestTheProgramWritesTheAPI(t *testing.T) {
	tr := newTree(t)
	tr.templates()
	tr.bundle("alice", "fix", siteWorkflow)
	tr.init()
	tr.commit("add the workflow")
	t.Chdir(tr.dir)
	code, _, errOut := runProgram(t, "--templates", "templates", "--workflows", "workflows", "--out", "docs")
	if code != exitOK {
		t.Fatalf("the program exits %d, want %d:\n%s", code, exitOK, errOut)
	}
	for _, path := range []string{"docs/api/index.json", "docs/api/alice/fix.json"} {
		if _, err := os.Stat(filepath.Join(tr.dir, path)); err != nil {
			t.Errorf("the program did not write %s: %v", path, err)
		}
	}
	index := readIndex(t, filepath.Join(tr.dir, "docs"))
	// The version is the contract with the consumer of the file: it
	// is written down here, and not read from the program.
	if index.Version != 1 {
		t.Errorf("the version of the index is %d, want 1", index.Version)
	}
	if index.Count != 1 {
		t.Errorf("the index counts %d workflows, want 1", index.Count)
	}
	if !equal(index.Namespaces, []string{"alice"}) {
		t.Errorf("the namespaces of the index are %v, want [alice]", index.Namespaces)
	}
	if len(index.Workflows) != 1 {
		t.Fatalf("the index lists %d workflows, want 1", len(index.Workflows))
	}
	if index.Workflows[0].Address != "alice/fix" {
		t.Errorf("the index lists %q, want %q", index.Workflows[0].Address, "alice/fix")
	}
}

// TestTheIndexListsEveryWorkflowInOrder checks the order of the index.
//
// The index is the list of the workflows of the repository, and the
// list has the order of the addresses: a consumer that reads the
// index twice reads the same workflows in the same order.
func TestTheIndexListsEveryWorkflowInOrder(t *testing.T) {
	tr := newTree(t)
	tr.templates()
	tr.bundle("zulu", "fix", siteWorkflow)
	tr.bundle("alice", "fix", siteWorkflow)
	tr.bundle("alice", "alpha", siteWorkflow)
	tr.init()
	tr.commit("add the workflows")
	t.Chdir(tr.dir)
	code, _, errOut := runProgram(t)
	if code != exitOK {
		t.Fatalf("the program exits %d, want %d:\n%s", code, exitOK, errOut)
	}
	index := readIndex(t, filepath.Join(tr.dir, "docs"))
	want := []string{"alice/alpha", "alice/fix", "zulu/fix"}
	if len(index.Workflows) != len(want) {
		t.Fatalf("the index lists %d workflows, want %d", len(index.Workflows), len(want))
	}
	if index.Count != len(want) {
		t.Errorf("the index counts %d workflows, want %d", index.Count, len(want))
	}
	for i, address := range want {
		if index.Workflows[i].Address != address {
			t.Errorf("the workflow %d of the index is %q, want %q", i, index.Workflows[i].Address, address)
		}
	}
	if !equal(index.Namespaces, []string{"alice", "zulu"}) {
		t.Errorf("the namespaces of the index are %v, want [alice zulu]", index.Namespaces)
	}
}

// TestTheIndexEntriesCarryNoJobs checks the shape of an entry.
//
// The index is a list of summaries: the jobs of a workflow and the
// actions it ships are in the detail file of the workflow, and the
// index does not repeat them.
func TestTheIndexEntriesCarryNoJobs(t *testing.T) {
	tr := newTree(t)
	tr.templates()
	tr.bundle("alice", "fix", siteWorkflow)
	tr.init()
	tr.commit("add the workflow")
	t.Chdir(tr.dir)
	code, _, errOut := runProgram(t)
	if code != exitOK {
		t.Fatalf("the program exits %d, want %d:\n%s", code, exitOK, errOut)
	}
	index := readJSON(t, filepath.Join(tr.dir, "docs", "api", "index.json"))
	entries, ok := index["workflows"].([]any)
	if !ok {
		t.Fatalf("the workflows of the index are %v, want a list", index["workflows"])
	}
	for _, item := range entries {
		entry, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("the entry %v is not an object", item)
		}
		if _, ok := entry["jobs"]; ok {
			t.Errorf("the entry of %v carries a jobs key", entry["address"])
		}
		if _, ok := entry["actions"]; ok {
			t.Errorf("the entry of %v carries an actions key", entry["address"])
		}
	}
}

// TestTheDetailHoldsTheWholeRecord checks the file of a workflow.
//
// The detail file holds the whole record of the workflow: the fields
// of the entry of the index, and the jobs with their steps and the
// actions the workflow ships.
func TestTheDetailHoldsTheWholeRecord(t *testing.T) {
	tr := newTree(t)
	tr.templates()
	tr.bundle("alice", "fix", siteWorkflow)
	tr.script("alice", "fix", "nightme-alice-report.sh")
	tr.init()
	tr.commit("add the workflow")
	t.Chdir(tr.dir)
	code, _, errOut := runProgram(t)
	if code != exitOK {
		t.Fatalf("the program exits %d, want %d:\n%s", code, exitOK, errOut)
	}
	index := readJSON(t, filepath.Join(tr.dir, "docs", "api", "index.json"))
	detail := readJSON(t, filepath.Join(tr.dir, "docs", "api", "alice", "fix.json"))
	entries, ok := index["workflows"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("the index holds %v, want one entry", index["workflows"])
	}
	entry, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatalf("the entry %v is not an object", entries[0])
	}
	for key, value := range entry {
		if !reflect.DeepEqual(detail[key], value) {
			t.Errorf("the detail holds %q = %v, want %v", key, detail[key], value)
		}
	}
	jobs, ok := detail["jobs"].([]any)
	if !ok || len(jobs) != 1 {
		t.Fatalf("the detail holds %v jobs, want one", detail["jobs"])
	}
	job, ok := jobs[0].(map[string]any)
	if !ok {
		t.Fatalf("the job %v is not an object", jobs[0])
	}
	steps, ok := job["steps"].([]any)
	if !ok || len(steps) != 1 {
		t.Fatalf("the job holds %v steps, want one", job["steps"])
	}
	step, ok := steps[0].(map[string]any)
	if !ok {
		t.Fatalf("the step %v is not an object", steps[0])
	}
	if step["id"] != "fix" {
		t.Errorf("the step is %q, want %q", step["id"], "fix")
	}
	actions, ok := detail["actions"].([]any)
	if !ok || len(actions) != 1 {
		t.Fatalf("the detail holds %v actions, want one", detail["actions"])
	}
}

// TestTheLinksAreAbsolute checks the links of a workflow.
//
// The links are absolute: a consumer on another domain uses them as
// they are.
func TestTheLinksAreAbsolute(t *testing.T) {
	tr := newTree(t)
	tr.templates()
	tr.bundle("alice", "fix", siteWorkflow)
	tr.init()
	tr.commit("add the workflow")
	t.Chdir(tr.dir)
	code, _, errOut := runProgram(t)
	if code != exitOK {
		t.Fatalf("the program exits %d, want %d:\n%s", code, exitOK, errOut)
	}
	index := readIndex(t, filepath.Join(tr.dir, "docs"))
	if len(index.Workflows) != 1 {
		t.Fatalf("the index lists %d workflows, want 1", len(index.Workflows))
	}
	want := map[string]string{
		"url":        "https://nightme-dev.github.io/workflows/alice/fix/",
		"detail":     "https://nightme-dev.github.io/workflows/api/alice/fix.json",
		"raw_source": "https://raw.githubusercontent.com/nightme-dev/workflows/main/workflows/alice/fix/workflow.yml",
		"source":     "https://github.com/nightme-dev/workflows/blob/main/workflows/alice/fix/workflow.yml",
	}
	got := map[string]string{
		"url":        index.Workflows[0].URL,
		"detail":     index.Workflows[0].Detail,
		"raw_source": index.Workflows[0].RawSource,
		"source":     index.Workflows[0].Source,
	}
	for name, value := range want {
		if got[name] != value {
			t.Errorf("the %s is %q, want %q", name, got[name], value)
		}
	}
}

// TestTheSiteURLNamesTheBaseOfTheLinks checks the flag that names the
// base URL of the links.
//
// The site may be published on another domain than the one it is
// built on, and the links of the API follow it.
func TestTheSiteURLNamesTheBaseOfTheLinks(t *testing.T) {
	tr := newTree(t)
	tr.templates()
	tr.bundle("alice", "fix", siteWorkflow)
	tr.init()
	tr.commit("add the workflow")
	t.Chdir(tr.dir)
	code, _, errOut := runProgram(t, "--site-url", "https://example.test/wf")
	if code != exitOK {
		t.Fatalf("the program exits %d, want %d:\n%s", code, exitOK, errOut)
	}
	index := readIndex(t, filepath.Join(tr.dir, "docs"))
	if len(index.Workflows) != 1 {
		t.Fatalf("the index lists %d workflows, want 1", len(index.Workflows))
	}
	want := map[string]string{
		"url":    "https://example.test/wf/alice/fix/",
		"detail": "https://example.test/wf/api/alice/fix.json",
	}
	got := map[string]string{
		"url":    index.Workflows[0].URL,
		"detail": index.Workflows[0].Detail,
	}
	for name, value := range want {
		if got[name] != value {
			t.Errorf("the %s is %q, want %q", name, got[name], value)
		}
	}
}

// TestTheSiteURLRefusesAValueThatIsNotAnAbsoluteURL checks the flag
// that names the base URL of the links.
//
// The links of the API are absolute, so a base that is not an absolute
// http or https URL is a value the command was called wrong with. A
// base that carries a query or a fragment cannot take a path under
// it, and a base that carries a user name would publish that name in
// every link.
func TestTheSiteURLRefusesAValueThatIsNotAnAbsoluteURL(t *testing.T) {
	for _, value := range []string{
		"example.test/wf",
		"/workflows",
		"ftp://example.test/wf",
		"https://",
		"",
		"https://example.test/wf?a=b",
		"https://example.test/wf?",
		"https://example.test/wf#",
		"https://user:secret@example.test/wf",
	} {
		t.Run(value, func(t *testing.T) {
			tr := newTree(t)
			tr.templates()
			tr.bundle("alice", "fix", siteWorkflow)
			tr.init()
			tr.commit("add the workflow")
			t.Chdir(tr.dir)
			code, _, errOut := runProgram(t, "--site-url", value)
			if code != exitUsage {
				t.Errorf("the site URL %q exits %d, want %d:\n%s", value, code, exitUsage, errOut)
			}
		})
	}
}

// TestTheSiteURLIsNormalized checks the base the links are built from.
//
// The base is the URL that was parsed, not the text that was typed:
// the links are built from the URL that was checked.
func TestTheSiteURLIsNormalized(t *testing.T) {
	cases := []struct {
		value string
		want  string
	}{
		{value: "https://example.test/wf", want: "https://example.test/wf"},
		{value: "https://example.test/wf/", want: "https://example.test/wf"},
		{value: "https://example.test", want: "https://example.test"},
		{value: "HTTPS://example.test/wf", want: "https://example.test/wf"},
	}
	for _, c := range cases {
		t.Run(c.value, func(t *testing.T) {
			got, err := siteURLOf(c.value)
			if err != nil {
				t.Fatalf("the site URL %q is refused: %v", c.value, err)
			}
			if got != c.want {
				t.Errorf("the site URL %q is %q, want %q", c.value, got, c.want)
			}
		})
	}
}

// TestTheIndexOfARepositoryWithoutAWorkflow checks the index of a
// repository that holds no workflow.
//
// The lists of the index are lists and not nothing: the data is read
// by other programs too.
func TestTheIndexOfARepositoryWithoutAWorkflow(t *testing.T) {
	tr := newTree(t)
	tr.templates()
	if err := os.MkdirAll(tr.workflows(), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", tr.workflows(), err)
	}
	tr.init()
	tr.commit("add the templates")
	t.Chdir(tr.dir)
	code, _, errOut := runProgram(t)
	if code != exitOK {
		t.Fatalf("the program exits %d, want %d:\n%s", code, exitOK, errOut)
	}
	raw := readFile(t, filepath.Join(tr.dir, "docs", "api", "index.json"))
	if !strings.Contains(string(raw), `"workflows": []`) {
		t.Errorf("the index does not hold an empty list of workflows:\n%s", raw)
	}
	index := readIndex(t, filepath.Join(tr.dir, "docs"))
	if index.Count != 0 {
		t.Errorf("the index counts %d workflows, want 0", index.Count)
	}
	if index.Namespaces == nil || len(index.Namespaces) != 0 {
		t.Errorf("the namespaces of the index are %v, want an empty list", index.Namespaces)
	}
	if index.GeneratedAt != "" {
		t.Errorf("the index says %q, want nothing: no commit touched the workflows", index.GeneratedAt)
	}
	if _, err := os.Stat(filepath.Join(tr.dir, "docs", "api", "alice", "fix.json")); err == nil {
		t.Error("the program wrote a detail file of a workflow that does not exist")
	}
}

// TestTheIndexSaysWhenTheWorkflowsLastChanged checks the date of the
// index.
//
// The date is the date of the newest commit that touched the
// workflows, and it is read by a query of its own: the changes page
// may hold no commit at all, and the index still says how fresh it
// is.
func TestTheIndexSaysWhenTheWorkflowsLastChanged(t *testing.T) {
	tr := newTree(t)
	tr.templates()
	tr.bundle("alice", "fix", siteWorkflow)
	tr.init()
	tr.commit("add the workflow")
	t.Chdir(tr.dir)
	code, _, errOut := runProgram(t)
	if code != exitOK {
		t.Fatalf("the program exits %d, want %d:\n%s", code, exitOK, errOut)
	}
	want := strings.TrimSpace(tr.git("log", "-1", "--format=%ad", "--date=short", "--", "workflows"))
	if want == "" {
		t.Fatal("the history does not know the commit that added the workflow")
	}
	index := readIndex(t, filepath.Join(tr.dir, "docs"))
	if index.GeneratedAt != want {
		t.Errorf("the index says %q, want %q", index.GeneratedAt, want)
	}
	code, _, errOut = runProgram(t, "--changes-limit", "0")
	if code != exitOK {
		t.Fatalf("the program exits %d, want %d:\n%s", code, exitOK, errOut)
	}
	index = readIndex(t, filepath.Join(tr.dir, "docs"))
	if index.GeneratedAt != want {
		t.Errorf("the index says %q, want %q", index.GeneratedAt, want)
	}
}

// TestTheIndexIsTheSameOnEveryRun checks that the build of the index
// is reproducible.
//
// The publish job of the site commits only when the site changed, so
// two builds of the same tree have to be the same bytes.
func TestTheIndexIsTheSameOnEveryRun(t *testing.T) {
	tr := newTree(t)
	tr.templates()
	tr.bundle("alice", "fix", siteWorkflow)
	tr.init()
	tr.commit("add the workflow")
	t.Chdir(tr.dir)
	code, _, errOut := runProgram(t)
	if code != exitOK {
		t.Fatalf("the program exits %d, want %d:\n%s", code, exitOK, errOut)
	}
	path := filepath.Join(tr.dir, "docs", "api", "index.json")
	first := readFile(t, path)
	code, _, errOut = runProgram(t)
	if code != exitOK {
		t.Fatalf("the program exits %d, want %d:\n%s", code, exitOK, errOut)
	}
	second := readFile(t, path)
	if !bytes.Equal(first, second) {
		t.Errorf("the index of the second run differs from the first:\n%s\n%s", first, second)
	}
}

// TestTheNamespaceAPIIsRefused checks the namespace that takes the
// path of the API.
//
// The machine-readable files of the site are served from "api", so a
// namespace that takes the name would take their prefix.
func TestTheNamespaceAPIIsRefused(t *testing.T) {
	tr := newTree(t)
	tr.bundle("api", "fix", siteWorkflow)
	tr.templates()
	tr.init()
	t.Chdir(tr.dir)
	code, _, errOut := runProgram(t)
	if code != exitInvalid {
		t.Errorf("the namespace %q exits %d, want %d:\n%s", "api", code, exitInvalid, errOut)
	}
	if !strings.Contains(errOut, "api") {
		t.Errorf("the error does not name the namespace %q:\n%s", "api", errOut)
	}
}

// TestThePagesHoldNothingButFrontMatter checks the pages of the site.
func TestThePagesHoldNothingButFrontMatter(t *testing.T) {
	tr := newTree(t)
	tr.templates()
	tr.bundle("alice", "fix", siteWorkflow)
	tr.init()
	tr.commit("add the workflow")
	t.Chdir(tr.dir)
	code, _, errOut := runProgram(t)
	if code != exitOK {
		t.Fatalf("the program exits %d, want %d:\n%s", code, exitOK, errOut)
	}
	cases := []struct {
		path  string
		front []string
	}{
		{path: "docs/index.md", front: []string{"layout: home", "permalink: /"}},
		{path: "docs/changes.md", front: []string{"layout: changes", "permalink: /changes/"}},
		{path: "docs/alice.md", front: []string{"layout: namespace", "namespace: alice", "permalink: /alice/"}},
		{path: "docs/alice/fix.md", front: []string{"layout: workflow", "address: alice/fix", "permalink: /alice/fix/"}},
	}
	for _, c := range cases {
		raw, err := os.ReadFile(filepath.Join(tr.dir, c.path))
		if err != nil {
			t.Errorf("read %s: %v", c.path, err)
			continue
		}
		text := string(raw)
		if !strings.HasPrefix(text, "---\n") || !strings.HasSuffix(text, "---\n") {
			t.Errorf("%s is not a page with front matter:\n%s", c.path, text)
			continue
		}
		body := strings.TrimSuffix(strings.TrimPrefix(text, "---\n"), "---\n")
		for _, field := range c.front {
			if !strings.Contains(body, field+"\n") {
				t.Errorf("the front matter of %s does not hold %q:\n%s", c.path, field, body)
			}
		}
		for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
			if line == "" {
				continue
			}
			known := false
			for _, field := range c.front {
				if field == line {
					known = true
				}
			}
			if !known {
				t.Errorf("the front matter of %s holds %q", c.path, line)
			}
		}
	}
}

// TestTheHistorySaysWhatChanged checks the changes of a real history.
//
// The site says which commit added a workflow and which one changed
// it: the history is the only thing that knows.
func TestTheHistorySaysWhatChanged(t *testing.T) {
	tr := newTree(t)
	tr.templates()
	tr.bundle("alice", "fix", siteWorkflow)
	tr.bundle("alice", "other", siteWorkflow)
	tr.init()
	added := tr.commit("add the workflows")
	tr.bundle("bob", "fix", siteWorkflow)
	tr.bundle("alice", "fix", siteWorkflow+"\n")
	tr.commit("change the workflows")
	t.Chdir(tr.dir)
	hist, err := openHistory("workflows")
	if err != nil {
		t.Fatalf("openHistory: %v", err)
	}
	changes, err := hist.changes(defaultChangesLimit)
	if err != nil {
		t.Fatalf("changes: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("the history holds %d commits, want 2: %+v", len(changes), changes)
	}
	if changes[0].SHA == added {
		t.Errorf("the newest commit is the one that added the workflows")
	}
	if !equal(changes[0].Added, []string{"bob/fix"}) {
		t.Errorf("the newest commit added %v, want [bob/fix]", changes[0].Added)
	}
	if !equal(changes[0].Modified, []string{"alice/fix"}) {
		t.Errorf("the newest commit changed %v, want [alice/fix]", changes[0].Modified)
	}
	if !equal(changes[1].Added, []string{"alice/fix", "alice/other"}) {
		t.Errorf("the first commit added %v, want [alice/fix alice/other]", changes[1].Added)
	}
	commit, ok := hist.lastCommit("alice/fix")
	if !ok {
		t.Fatal("the history does not say which commit changed the bundle last")
	}
	if commit.SHA != changes[0].SHA {
		t.Errorf("the last change of the bundle is %q, want %q", commit.SHA, changes[0].SHA)
	}
	if commit.Date == "" || commit.URL == "" {
		t.Errorf("the last change of the bundle is not complete: %+v", commit)
	}
}

// TestTheProgramFailsOnAMissingTemplateDirectory checks the exit code
// of a path that cannot be read.
func TestTheProgramFailsOnAMissingTemplateDirectory(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", siteWorkflow)
	tr.init()
	t.Chdir(tr.dir)
	code, _, errOut := runProgram(t, "--templates", "templates")
	if code != exitUnreadable {
		t.Errorf("a missing directory of templates exits %d, want %d:\n%s", code, exitUnreadable, errOut)
	}
}

// TestTheProgramRefusesToOverwriteTheRepository checks that the site
// source cannot be written over the workflows or the templates.
func TestTheProgramRefusesToOverwriteTheRepository(t *testing.T) {
	for _, out := range []string{"workflows", "templates", ".", "workflows/alice", "templates/_layouts"} {
		t.Run(out, func(t *testing.T) {
			tr := newTree(t)
			tr.templates()
			tr.bundle("alice", "fix", siteWorkflow)
			tr.init()
			tr.commit("add the workflow")
			t.Chdir(tr.dir)
			code, _, errOut := runProgram(t, "--out", out)
			if code != exitUsage {
				t.Errorf("the output directory %q exits %d, want %d:\n%s", out, code, exitUsage, errOut)
			}
			if _, err := os.Stat(filepath.Join(tr.dir, "workflows", "alice", "fix", workflowFile)); err != nil {
				t.Errorf("the workflows are gone: %v", err)
			}
		})
	}
}

// runProgram runs the program in the tree and returns the exit code
// and what it wrote to the error output.
func runProgram(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut strings.Builder
	code := run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

// equal says whether two lists of strings hold the same strings.
func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
