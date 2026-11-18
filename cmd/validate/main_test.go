package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cnlangzi/nightme/pkg/wfe"
)

// A workflow that validates: a title, one job, one step.
const validWorkflow = `title: a workflow
jobs:
  main:
    steps:
      - id: one
        message: hello
`

// tree is a repository under test: a workflows directory and the README
// that holds the catalog.
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

// writeScript writes an executable script of the tree.
func (tr *tree) writeScript(path string) string {
	tr.t.Helper()
	full := filepath.Join(tr.dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		tr.t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		tr.t.Fatalf("write %s: %v", full, err)
	}
	return full
}

// bundle writes a bundle with a workflow file.
func (tr *tree) bundle(ns, name, workflow string) {
	tr.t.Helper()
	tr.write(filepath.Join("workflows", ns, name, workflowFile), workflow)
}

// mkdir makes a directory of the tree.
func (tr *tree) mkdir(path string) {
	tr.t.Helper()
	if err := os.MkdirAll(filepath.Join(tr.dir, path), 0o755); err != nil {
		tr.t.Fatalf("mkdir %s: %v", path, err)
	}
}

// readme writes the README, with a catalog of the given addresses.
func (tr *tree) readme(addresses ...string) {
	tr.t.Helper()
	var b strings.Builder
	b.WriteString("# workflows\n\n## Workflows\n\n")
	b.WriteString("| Address | What it does |\n|---|---|\n")
	for _, a := range addresses {
		b.WriteString("| `" + a + "` | a workflow |\n")
	}
	b.WriteString("\n## Something else\n\nprose.\n")
	tr.write("README.md", b.String())
}

// root is the workflows directory of the tree.
func (tr *tree) root() string {
	return filepath.Join(tr.dir, "workflows")
}

// validate validates the tree and returns the problems it found.
func (tr *tree) validate() []wfe.Diagnostic {
	tr.t.Helper()
	diags, err := validate(tr.root())
	if err != nil {
		tr.t.Fatalf("validate: %v", err)
	}
	return diags
}

// report renders the problems the way the program prints them.
func report(diags []wfe.Diagnostic) string {
	var b strings.Builder
	for _, d := range diags {
		b.WriteString(d.Error())
		b.WriteString("\n")
	}
	return b.String()
}

// wantProblem fails the test unless the problems hold the text.
func wantProblem(t *testing.T, diags []wfe.Diagnostic, text string) {
	t.Helper()
	got := report(diags)
	if !strings.Contains(got, text) {
		t.Errorf("the problems do not hold %q:\n%s", text, got)
	}
}

// wantNoProblem fails the test when the problems hold the text.
func wantNoProblem(t *testing.T, diags []wfe.Diagnostic, text string) {
	t.Helper()
	got := report(diags)
	if strings.Contains(got, text) {
		t.Errorf("the problems hold %q:\n%s", text, got)
	}
}

func TestValidRepositoryHasNoProblems(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.readme("alice/fix")
	if diags := tr.validate(); len(diags) != 0 {
		t.Errorf("a valid repository has problems:\n%s", report(diags))
	}
}

func TestBundleWithoutWorkflowFile(t *testing.T) {
	tr := newTree(t)
	tr.write("workflows/alice/fix/README.md", "notes\n")
	tr.readme("alice/fix")
	wantProblem(t, tr.validate(), "the bundle holds no workflow.yml")
}

func TestBundleWithAnotherWorkflowFile(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.write("workflows/alice/fix/other.yml", validWorkflow)
	tr.readme("alice/fix")
	wantProblem(t, tr.validate(), "the workflow file of a bundle is named workflow.yml")
}

func TestYamlExtensionIsReported(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.write("workflows/alice/fix/other.yaml", validWorkflow)
	tr.readme("alice/fix")
	wantProblem(t, tr.validate(), "the repository uses the .yml extension")
}

func TestUnknownKeyIsReported(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow+"meta:\n  author: alice\n")
	tr.readme("alice/fix")
	wantProblem(t, tr.validate(), `unknown key "meta"`)
}

func TestSchemaViolationIsReported(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", "jobs:\n  main:\n    steps:\n      - id: one\n        message: hello\n")
	tr.readme("alice/fix")
	wantProblem(t, tr.validate(), "title is required")
}

func TestNamespaceShape(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice_1", "fix", validWorkflow)
	tr.readme()
	wantProblem(t, tr.validate(), "is not a GitHub account name")
}

func TestNameShape(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "Fix", validWorkflow)
	tr.readme()
	wantProblem(t, tr.validate(), "is not lowercase kebab-case")
}

func TestIdentityCollision(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix-x", validWorkflow)
	tr.bundle("alice-fix", "x", validWorkflow)
	tr.readme("alice/fix-x", "alice-fix/x")
	diags := tr.validate()
	wantProblem(t, diags, `the identity "alice-fix-x" is used by more than one workflow`)
	if got := strings.Count(report(diags), `the identity "alice-fix-x"`); got != 2 {
		t.Errorf("the identity is reported %d times, want 2:\n%s", got, report(diags))
	}
}

func TestUseNamesNoAction(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", `title: a workflow
jobs:
  main:
    steps:
      - id: one
        use: nowhere
`)
	tr.readme("alice/fix")
	diags := tr.validate()
	wantProblem(t, diags, `the step uses the action "nowhere"`)
	// The problem points at the step that names the action.
	wantProblem(t, diags, "workflow.yml:6:9: ")
}

func TestTypeErrorKeepsTheActionNames(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", `title: a workflow
args: "not a list"
jobs:
  main:
    steps:
      - id: one
        use: alice-fix-report
`)
	tr.writeScript("workflows/alice/fix/actions/alice-fix-report.sh")
	tr.readme("alice/fix")
	diags := tr.validate()
	wantProblem(t, diags, "`args:` must be a list")
	wantNoProblem(t, diags, "is used by no step")
}

func TestUseNamesABuiltinAction(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", `title: a workflow
jobs:
  main:
    steps:
      - id: one
        use: notify
`)
	tr.readme("alice/fix")
	if diags := tr.validate(); len(diags) != 0 {
		t.Errorf("a built-in action is a valid action:\n%s", report(diags))
	}
}

func TestUseNamesABundledAction(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", `title: a workflow
jobs:
  main:
    steps:
      - id: one
        use: alice-fix-report
`)
	tr.writeScript("workflows/alice/fix/actions/alice-fix-report.sh")
	tr.readme("alice/fix")
	if diags := tr.validate(); len(diags) != 0 {
		t.Errorf("a bundled action is a valid action:\n%s", report(diags))
	}
}

func TestUnreferencedAction(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.writeScript("workflows/alice/fix/actions/alice-fix-report.sh")
	tr.readme("alice/fix")
	wantProblem(t, tr.validate(), `the bundled action "alice-fix-report" is used by no step`)
}

func TestActionThatIsNotAScript(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.write("workflows/alice/fix/actions/alice-fix-report.txt", "not a script\n")
	tr.readme("alice/fix")
	wantProblem(t, tr.validate(), "a bundled action is a .sh script")
}

func TestActionThatIsNotExecutable(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.write("workflows/alice/fix/actions/alice-fix-report.sh", "#!/bin/sh\n")
	tr.readme("alice/fix")
	wantProblem(t, tr.validate(), "is not executable")
}

func TestActionNameInTwoBundles(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.bundle("bob", "fix", validWorkflow)
	tr.writeScript("workflows/alice/fix/actions/report.sh")
	tr.writeScript("workflows/bob/fix/actions/report.sh")
	tr.readme("alice/fix", "bob/fix")
	wantProblem(t, tr.validate(), `the action "report" is shipped by more than one bundle`)
}

func TestCatalogEntryWithoutBundle(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.readme("alice/fix", "bob/fix")
	wantProblem(t, tr.validate(), `the catalog lists "bob/fix", which no workflow has`)
}

func TestBundleWithoutCatalogEntry(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.readme()
	wantProblem(t, tr.validate(), `the workflow "alice/fix" is not listed in the README catalog`)
}

func TestCatalogEntryWithoutAnyBundle(t *testing.T) {
	tr := newTree(t)
	tr.mkdir("workflows")
	tr.readme("alice/fix")
	wantProblem(t, tr.validate(), `the catalog lists "alice/fix", which no workflow has`)
}

func TestCatalogEntryThatIsNotAnAddress(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.write("README.md", "# workflows\n\n## Workflows\n\n| `Alice/Fix` |\n")
	wantProblem(t, tr.validate(), `the catalog entry "Alice/Fix" is not a valid address`)
}

func TestExitCodes(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.readme("alice/fix")

	var out, errOut strings.Builder
	if code := run([]string{tr.root()}, &out, &errOut); code != exitOK {
		t.Errorf("a valid repository exits %d, want %d", code, exitOK)
	}
	if out.String() != "" {
		t.Errorf("a valid repository prints %q, want nothing", out.String())
	}

	tr.bundle("alice", "broken", "jobs: {}\n")
	tr.readme("alice/fix", "alice/broken")
	out.Reset()
	errOut.Reset()
	if code := run([]string{tr.root()}, &out, &errOut); code != exitInvalid {
		t.Errorf("a broken workflow exits %d, want %d", code, exitInvalid)
	}
	if out.Len() == 0 {
		t.Error("a broken workflow prints nothing")
	}

	out.Reset()
	errOut.Reset()
	if code := run([]string{filepath.Join(tr.dir, "nowhere")}, &out, &errOut); code != exitUnreadable {
		t.Errorf("a path that does not exist exits %d, want %d", code, exitUnreadable)
	}
}

func TestProblemsAreOneLineEach(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow+"meta: {}\n")
	tr.readme()
	for _, d := range tr.validate() {
		if strings.Contains(d.Error(), "\n") {
			t.Errorf("a problem spans more than one line: %q", d.Error())
		}
	}
}

func TestBundleOnItsOwn(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	diags, err := validate(filepath.Join(tr.dir, "workflows", "alice", "fix"))
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	wantNoProblem(t, diags, "the bundle holds no workflow.yml")
}
