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

// tree is a repository under test: a workflows directory.
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

// validateChanged validates the bundles the values name and returns the
// problems it found.
func (tr *tree) validateChanged(values ...string) []wfe.Diagnostic {
	tr.t.Helper()
	diags, err := validateChanged(tr.root(), values)
	if err != nil {
		tr.t.Fatalf("validateChanged: %v", err)
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

// runProgram runs the program and returns the exit code, what it
// printed and what it wrote to the error output.
func runProgram(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut strings.Builder
	code := run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestValidRepositoryHasNoProblems(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	if diags := tr.validate(); len(diags) != 0 {
		t.Errorf("a valid repository has problems:\n%s", report(diags))
	}
}

func TestBundleWithoutWorkflowFile(t *testing.T) {
	tr := newTree(t)
	tr.write("workflows/alice/fix/README.md", "notes\n")
	wantProblem(t, tr.validate(), "the bundle holds no workflow.yml")
}

func TestBundleWithAnotherWorkflowFile(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.write("workflows/alice/fix/other.yml", validWorkflow)
	wantProblem(t, tr.validate(), "the workflow file of a bundle is named workflow.yml")
}

func TestYamlExtensionIsReported(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.write("workflows/alice/fix/other.yaml", validWorkflow)
	wantProblem(t, tr.validate(), "the repository uses the .yml extension")
}

func TestUnknownKeyIsReported(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow+"meta:\n  author: alice\n")
	wantProblem(t, tr.validate(), `unknown key "meta"`)
}

func TestSchemaViolationIsReported(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", "jobs:\n  main:\n    steps:\n      - id: one\n        message: hello\n")
	wantProblem(t, tr.validate(), "title is required")
}

func TestNamespaceShape(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice_1", "fix", validWorkflow)
	wantProblem(t, tr.validate(), "is not a GitHub account name")
}

func TestNameShape(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "Fix", validWorkflow)
	wantProblem(t, tr.validate(), "is not lowercase kebab-case")
}

func TestIdentityCollision(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix-x", validWorkflow)
	tr.bundle("alice-fix", "x", validWorkflow)
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
	if diags := tr.validate(); len(diags) != 0 {
		t.Errorf("a bundled action is a valid action:\n%s", report(diags))
	}
}

func TestUseNamesAnArgument(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", `title: a workflow
args:
  - name: which
    default: notify
jobs:
  main:
    steps:
      - id: one
        use: ${{ args.which }}
`)
	if diags := tr.validate(); len(diags) != 0 {
		t.Errorf("an action named by an argument is a valid action:\n%s", report(diags))
	}
}

func TestUseNamesAnArgumentBoundToABundledAction(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", `title: a workflow
args:
  - name: which
    default: alice-fix-report
jobs:
  main:
    steps:
      - id: one
        use: ${{ args.which }}
`)
	tr.writeScript("workflows/alice/fix/actions/alice-fix-report.sh")
	if diags := tr.validate(); len(diags) != 0 {
		t.Errorf("an action named by an argument is a valid action:\n%s", report(diags))
	}
}

func TestUseNamesAnExpressionThatStays(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", `title: a workflow
jobs:
  main:
    steps:
      - id: one
        use: ${{ steps.other.outputs.action }}
`)
	tr.writeScript("workflows/alice/fix/actions/alice-fix-report.sh")
	diags := tr.validate()
	// The action is chosen at run time: the validator cannot say
	// which one it is, so it says nothing about the wiring.
	wantNoProblem(t, diags, "is used by no step")
	wantNoProblem(t, diags, "which is neither a built-in action")
}

func TestUnreferencedAction(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.writeScript("workflows/alice/fix/actions/alice-fix-report.sh")
	wantProblem(t, tr.validate(), `the bundled action "alice-fix-report" is used by no step`)
}

func TestActionThatIsNotAScript(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.write("workflows/alice/fix/actions/alice-fix-report.txt", "not a script\n")
	wantProblem(t, tr.validate(), "a bundled action is a .sh script")
}

func TestActionThatIsNotExecutable(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.write("workflows/alice/fix/actions/alice-fix-report.sh", "#!/bin/sh\n")
	wantProblem(t, tr.validate(), "is not executable")
}

func TestActionNameInTwoBundles(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.bundle("bob", "fix", validWorkflow)
	tr.writeScript("workflows/alice/fix/actions/report.sh")
	tr.writeScript("workflows/bob/fix/actions/report.sh")
	wantProblem(t, tr.validate(), `the action "report" is shipped by more than one bundle`)
}

func TestStrayWorkflowFileDoesNotHideTheTree(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.write("workflows/workflow.yml", validWorkflow)
	diags := tr.validate()
	wantProblem(t, diags, "a workflow lives in a bundle directory")
	// The tree is still checked: a bundle that does not validate is
	// still reported.
	tr.bundle("bob", "fix", "jobs: {}\n")
	wantProblem(t, tr.validate(), "jobs is required")
}

func TestUnparseableWorkflowFileDoesNotReportUnusedActions(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", "title: t\njobs:\n  m:\n   steps:\n  bad\n")
	tr.writeScript("workflows/alice/fix/actions/alice-fix-report.sh")
	diags := tr.validate()
	wantProblem(t, diags, "could not find expected")
	wantNoProblem(t, diags, "is used by no step")
}

func TestReadmeIsNotRead(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.write("README.md", "# workflows\n\nThe workflows live under `workflows/`.\n")
	if diags := tr.validate(); len(diags) != 0 {
		t.Errorf("the README is not part of the format:\n%s", report(diags))
	}
}

func TestExitCodes(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)

	var out, errOut strings.Builder
	if code := run([]string{tr.root()}, &out, &errOut); code != exitOK {
		t.Errorf("a valid repository exits %d, want %d", code, exitOK)
	}
	if out.String() != "" {
		t.Errorf("a valid repository prints %q, want nothing", out.String())
	}

	tr.bundle("alice", "broken", "jobs: {}\n")
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

// The scoped mode: only the bundles that are named are checked.

func TestChangedChecksOnlyTheNamedBundle(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", "jobs: {}\n")
	tr.bundle("bob", "build", validWorkflow)

	// The bundle that is valid is valid, and the broken one is not
	// its business.
	code, out, _ := runProgram(t, "--changed", "bob/build", tr.root())
	if code != exitOK {
		t.Errorf("a valid bundle exits %d, want %d:\n%s", code, exitOK, out)
	}
	if out != "" {
		t.Errorf("a valid bundle prints %q, want nothing", out)
	}

	code, out, _ = runProgram(t, "--changed", "alice/fix", tr.root())
	if code != exitInvalid {
		t.Errorf("a broken bundle exits %d, want %d", code, exitInvalid)
	}
	wantProblem(t, tr.validateChanged("alice/fix"), "jobs is required")
	if !strings.Contains(out, "jobs is required") {
		t.Errorf("the problem of the bundle is not printed:\n%s", out)
	}
}

func TestChangedChecksTheShapeOfTheName(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "Fix", validWorkflow)
	wantProblem(t, tr.validateChanged("alice/Fix"), `the name "Fix" is not lowercase kebab-case`)
}

func TestChangedChecksTheShapeOfTheNamespace(t *testing.T) {
	tr := newTree(t)
	tr.bundle("bad_name", "fix", validWorkflow)
	wantProblem(t, tr.validateChanged("bad_name/fix"),
		`the namespace "bad_name" is not a GitHub account name: letters, digits and dashes`)
}

func TestChangedReportsAFileUnderTheRoot(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.write("workflows/stray.yml", validWorkflow)
	diags := tr.validateChanged("stray.yml")
	wantProblem(t, diags, "a workflow lives in a bundle directory: workflows/<ns>/<name>/workflow.yml")
	// The layout is the problem: the shape of the path is not.
	wantNoProblem(t, diags, "is not lowercase kebab-case")
	wantNoProblem(t, diags, "is not a GitHub account name")
}

func TestChangedReportsAFileUnderANamespace(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	tr.write("workflows/alice/stray.yml", validWorkflow)
	diags := tr.validateChanged("alice/stray.yml")
	wantProblem(t, diags, "a namespace directory holds bundle directories only: workflows/<ns>/<name>/")
	// The layout is the problem: the shape of the path is not.
	wantNoProblem(t, diags, "is not lowercase kebab-case")
	wantNoProblem(t, diags, "is not a GitHub account name")
}

func TestChangedIgnoresADeletedBundle(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	code, out, _ := runProgram(t, "--changed", "alice/gone", tr.root())
	if code != exitOK {
		t.Errorf("a deleted bundle exits %d, want %d:\n%s", code, exitOK, out)
	}
	if out != "" {
		t.Errorf("a deleted bundle prints %q, want nothing", out)
	}
}

func TestChangedChecksTheShapeOfANamespaceDirectory(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	if diags := tr.validateChanged("alice"); len(diags) != 0 {
		t.Errorf("a namespace directory with a good name has problems:\n%s", report(diags))
	}

	tr.bundle("bad_name", "fix", validWorkflow)
	wantProblem(t, tr.validateChanged("bad_name"),
		`the namespace "bad_name" is not a GitHub account name: letters, digits and dashes`)
}

func TestChangedChecksTheRulesOfTheBundle(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", `title: a workflow
jobs:
  main:
    steps:
      - id: one
        use: nowhere
`)
	tr.writeScript("workflows/alice/fix/actions/unused.sh")
	tr.write("workflows/alice/fix/other.yaml", validWorkflow)
	diags := tr.validateChanged("alice/fix")
	wantProblem(t, diags, `the step uses the action "nowhere"`)
	wantProblem(t, diags, `the bundled action "unused" is used by no step`)
	wantProblem(t, diags, "the repository uses the .yml extension")
}

func TestChangedDoesNotConsultTheTree(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix-x", validWorkflow)
	tr.bundle("alice-fix", "x", validWorkflow)
	tr.writeScript("workflows/alice/fix/actions/report.sh")
	tr.writeScript("workflows/alice-fix/x/actions/report.sh")
	// The identity and the action name collide, but they are rules of
	// the tree, not of a bundle.
	if diags := tr.validateChanged("alice/fix-x"); len(diags) != 0 {
		t.Errorf("the scoped mode reports the rules of the tree:\n%s", report(diags))
	}
	// The full mode still reports them.
	wantProblem(t, tr.validate(), `the identity "alice-fix-x" is used by more than one workflow`)
	wantProblem(t, tr.validate(), `the action "report" is shipped by more than one bundle`)
}

func TestChangedTheFlagMayBeGivenMoreThanOnce(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", "jobs: {}\n")
	tr.bundle("bob", "build", validWorkflow)

	code, out, _ := runProgram(t, "--changed", "bob/build", "--changed", "alice/fix", tr.root())
	if code != exitInvalid {
		t.Errorf("a broken bundle exits %d, want %d", code, exitInvalid)
	}
	if !strings.Contains(out, "jobs is required") {
		t.Errorf("the problem of the bundle is not printed:\n%s", out)
	}

	// The same bundles, in one value.
	code, out, _ = runProgram(t, "--changed", "bob/build,alice/fix", tr.root())
	if code != exitInvalid {
		t.Errorf("a broken bundle exits %d, want %d", code, exitInvalid)
	}
	if !strings.Contains(out, "jobs is required") {
		t.Errorf("the problem of the bundle is not printed:\n%s", out)
	}
}

func TestChangedAnEmptyValueValidatesNothing(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", "jobs: {}\n")
	code, out, _ := runProgram(t, "--changed", "", tr.root())
	if code != exitOK {
		t.Errorf("an empty value exits %d, want %d:\n%s", code, exitOK, out)
	}
	if out != "" {
		t.Errorf("an empty value prints %q, want nothing", out)
	}
}

func TestChangedWithoutTheRootUsesTheDefault(t *testing.T) {
	t.Chdir(t.TempDir())
	// The root is the workflows directory of the working directory,
	// and there is none.
	code, out, errOut := runProgram(t, "--changed", "alice/fix")
	if code != exitUnreadable {
		t.Errorf("a root that does not exist exits %d, want %d:\n%s %s", code, exitUnreadable, out, errOut)
	}
	if !strings.Contains(errOut, defaultRoot) {
		t.Errorf("the error does not name the default root %q:\n%s", defaultRoot, errOut)
	}
}

func TestChangedOnASingleBundleIsAUsageError(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	code, out, _ := runProgram(t, "--changed", "alice/fix",
		filepath.Join(tr.dir, "workflows", "alice", "fix"))
	if code != exitUsage {
		t.Errorf("a bundle as the root exits %d, want %d", code, exitUsage)
	}
	if out != "" {
		t.Errorf("a bundle as the root prints %q, want nothing", out)
	}
}

func TestChangedNamesABundleByItsFirstTwoSegments(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", "jobs: {}\n")
	diags := tr.validateChanged("alice/fix/actions/report.sh")
	wantProblem(t, diags, "jobs is required")
}

func TestChangedChecksTheSameBundleOnce(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", "jobs: {}\n")
	// The same bundle, named two ways: it is checked once.
	diags := tr.validateChanged("alice/fix", "alice/fix/workflow.yml", "alice/fix/")
	if got := strings.Count(report(diags), "jobs is required"); got != 1 {
		t.Errorf("the problem is reported %d times, want 1:\n%s", got, report(diags))
	}
}

func TestChangedOnASingleBundleSaysWhy(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	code, _, errOut := runProgram(t, "--changed", "alice/fix",
		filepath.Join(tr.dir, "workflows", "alice", "fix"))
	if code != exitUsage {
		t.Errorf("a bundle as the root exits %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "the root is a bundle") {
		t.Errorf("the reason is not shown:\n%s", errOut)
	}
}

func TestChangedRejectsAPathOutsideTheRoot(t *testing.T) {
	tr := newTree(t)
	tr.bundle("alice", "fix", validWorkflow)
	code, _, errOut := runProgram(t, "--changed", "../other/bundle", tr.root())
	if code != exitUsage {
		t.Errorf("a path outside the root exits %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "outside the workflows directory") {
		t.Errorf("the reason is not shown:\n%s", errOut)
	}
}
