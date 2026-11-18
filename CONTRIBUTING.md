# Contributing

A workflow is a YAML file the personal agent (`/me`) runs. This repository
holds the ones the community publishes; the rules below are what a workflow
has to look like, and what a reviewer has to judge.

There are no per-workflow version numbers: the repository's git refs are the
version axis, so a workflow is installed at a commit, not at a version.

## The format

### The layout

Every workflow lives in a directory of its own:

```
workflows/<ns>/<name>/workflow.yml
```

- `<ns>` is a namespace: a GitHub account name, matching
  `^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$` — letters, digits and dashes, starting
  with a letter or a digit. You choose it freely; the reviewer decides whether
  it legitimately belongs to you.
- `<name>` is a lowercase kebab-case short name, matching
  `^[a-z0-9]+(?:-[a-z0-9]+)*$`.
- The directory holds exactly one workflow file, and the file is named
  `workflow.yml`.

The **address** of a workflow is `<ns>/<name>`. The address is the path, so
there is no table anywhere that maps an address to a file.

The **identity** of a workflow is `<ns>-<name>`. It is the name the workflow
carries once it is installed next to other workflows, so two workflows in this
repository may not share one.

The repository uses the `.yml` extension only. A `.yaml` file is the same
thing spelled differently, and the two spellings drift apart.

### The workflow file

A workflow file holds only the keys of the nightme workflow schema:

| Key | What it is |
|---|---|
| `title` | what the workflow is for |
| `args` | the arguments the workflow takes |
| `instruct` | whether a step's message is an instruction to the agent |
| `workspaces` | the directories the workflow runs in |
| `on` | the triggers — parsed and displayed, nothing fires them yet |
| `jobs` | the jobs, and the steps of every job |

Nothing else. A `meta:` block, a `version:`, a `description:` — every key the
schema does not have is reported as an unknown key, and the file does not
validate. The rules themselves live in nightme (`pkg/wfe`), so the validator
here and the runtime agree by construction.

### The actions

Attachments are declared implicitly. A step that runs an action names it:

```yaml
- id: tell
  use: notify
  with:
    text: "done"
```

A `use:` step may name one of the built-in actions nightme registers itself:

| Built-in | What it does |
|---|---|
| `notify` | feishu / slack / webhook |
| `email` | SMTP |
| `github_pr_comment` | a comment on a pull request |
| `github_issue` | a comment on an issue |

Every other name must be an action the workflow ships itself. A bundled action
is a script in the `actions/` directory of the same bundle:

```
workflows/<ns>/<name>/actions/<action>.sh
```

- The action's name is the name of the file without the extension: a file
  `actions/nightme-coder-report.sh` provides the action `nightme-coder-report`,
  and a step runs it with `use: nightme-coder-report`.
- The convention is to name the file `<ns>-<name>-<action>.sh` — the workflow
  `nightme/coder` ships `actions/nightme-coder-report.sh`. It is a convention,
  not a rule the validator enforces, but it is why action names do not collide:
  the identity of the workflow is already part of the name.
- Every script has to be executable — the executable bit is part of the file,
  so `chmod +x` it before you commit.
- Every script has to be used. A script no step names is reported: it is
  either a leftover or a step that was renamed.
- Action names are unique across the repository. Two bundles that ship the
  same name are both reported: a `use:` step says which action it means, and
  the name alone does not.

An action name may come from an argument — `use: ${{ args.which }}` — the way
any other field of a step may. The runtime binds the arguments of a workflow
before it runs it, so the name is resolved to the action it names. The
validator binds the arguments the workflow declares as well, so an action
named by an argument is checked like any other one; an action named by
something the run has not resolved yet is left alone.

A bundle may also carry a `README.md`. It is addressed to humans, it is not
part of the format, and the validator ignores it.

### The catalog

The `## Workflows` section of [README.md](README.md) lists every workflow of
the repository, one table row per workflow, with the address in backticks in
the first column. The catalog is written by hand and the validator checks it
against the tree: a workflow that is not listed is reported, and so is an
entry that names no workflow.

## The process

1. Fork the repository and make a branch.
2. Add your bundle under `workflows/<ns>/<name>/`, with the workflow file and
   whatever it ships.
3. Add your address to the `## Workflows` section of the README.
4. Run the validator: `go run ./cmd/validate ./workflows`. It has to exit 0.
5. Open a pull request. CI runs the same command and has to pass.

The pull request template asks the questions a reviewer has to answer.

## What the machine checks, and what the reviewer decides

The machine decides the objective facts. It cannot tell whether a workflow is
safe, whether a namespace belongs to you, or whether a name is apt — so it
does not try, and it does not report anything it cannot know.

| The machine (objective) | The reviewer (judgment) |
|---|---|
| the workflow file parses and satisfies the schema | is the namespace the author's own? |
| the directory holds exactly one `workflow.yml` | is the name apt? |
| the names are shaped right, the identity is unique | what will the `shell:` steps execute? |
| `use:` names resolve to a built-in action or a bundled script | what will they write or delete? |
| bundled scripts are executable and all of them are referenced | should this be several workflows? |
| the README catalog matches the tree | is it safe to merge? |

The namespace is the pull request author's own choice, and no namespace is
reserved: this repository is community co-created. Whether the namespace
legitimately belongs to the author is the reviewer's call — the validator
reports the shape of a name, never who owns it.

## The reviewer's checklist

- Is the namespace the author's own?
- Is the name apt — does it say what the workflow does?
- What will the `shell:` steps execute?
- What will they write or delete?
- Should this be several workflows instead of one?
- Is it safe to merge?

## The CI pipeline

The CI pipeline runs one job: the validator. It also runs the tests of the
validator itself, so a change that breaks them is caught before it breaks the
check.

The pipeline and the validator are protected by
[CODEOWNERS](.github/CODEOWNERS): a pull request that changes either of them
needs a review from a code owner. The validator is protected for the same
reason as the pipeline — a change that made it pass everything would defeat
the check.

A CODEOWNERS rule only takes effect once the repository setting **Settings →
Repositories → Settings → Require review from Code Owners** is enabled. Until
a maintainer turns it on, a pull request can change the CI pipeline itself.
