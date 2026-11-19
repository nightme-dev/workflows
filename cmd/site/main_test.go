package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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

// siteOf builds the site of the bundles with the history.
func siteOf(t *testing.T, bundles []bundle, hist history) *siteData {
	t.Helper()
	data, err := buildSite(bundles, hist, defaultChangesLimit)
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
// A step is a message, a shell command, an action or a wait. A step
// that is none of them has no kind.
func TestTheKindOfAStep(t *testing.T) {
	cases := []struct {
		name string
		step string
		want string
	}{
		{name: "a message", step: "message: hello", want: "message"},
		{name: "a shell command", step: "shell: ls", want: "shell"},
		{name: "an action", step: "use: nightme-report", want: "use"},
		{name: "a wait", step: "wait: 5m", want: "wait"},
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
