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
named by an argument is checked like any other one. An action named by
something the run has not resolved yet is left alone — and then no bundled
action is reported as unused either, because the wiring cannot be judged.

A bundle may also carry a `README.md`. It is addressed to humans, it is not
part of the format, and the validator ignores it.

## The process

1. Fork the repository and make a branch.
2. Add your bundle under `workflows/<ns>/<name>/`, with the workflow file and
   whatever it ships.
3. Run the validator: `go run ./cmd/validate ./workflows`. It has to exit 0.
4. Open a pull request. CI validates the bundles the pull request changed.

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

## The site

The repository has an index: <https://nightme-dev.github.io/workflows/>. It
shows every workflow of the repository, and it is built by the CI on every
push to `main` — the site is never written by hand.

What a workflow says on the site comes from the workflow itself:

- the `title` is the heading of the page;
- the `args` are the arguments the workflow takes, with their defaults;
- the `on` block is the list of the triggers;
- the `workspaces` are the directories the workflow runs in;
- the `jobs` and their `steps` are the steps of the workflow, with their
  notes.

A bundle may also carry a `README.md`, and its **first paragraph** is the
summary of the workflow on the site. The first paragraph is the first block
of text of the file that is not a heading, so a README that starts with a
title and then explains the workflow in a paragraph shows that paragraph as
the summary. A README that has no paragraph shows no summary at all: the
site leaves the section out rather than showing an empty one.

The pages of the site are generated from the workflows and from the templates
in `templates/`; nothing of the site is committed to the repository. A
workflow that is taken away takes its page with it: the site is built from
scratch every time, and the page of a workflow that no longer exists is
simply not there. The layout of the site is reviewed like everything else: a
change of the templates is a pull request.

## The CI pipeline

The CI pipeline runs one job: the validator. It also runs the tests of the
validator itself, so a change that breaks them is caught before it breaks the
check.

The job validates the bundles the pull request changed, and only those: a
pull request is judged on what it changed, and a workflow nobody touched
cannot fail it. The rules that need the whole tree — the unique identity,
the unique action name — are not among them: they are checked by the full
local run. The tests of the validator run on every push and every pull
request, whether a workflow changed or not.

A second workflow publishes the site: it builds the index of the workflows,
builds the site with Jekyll, builds the search index and publishes the
result to GitHub Pages. It runs on a push to `main` that touches a workflow
or the site itself, and on a manual run. A push that touches none of them
skips it: the site that is published is the site of the last build.

The pipeline is part of the repository like everything else: a pull request
that changes it is reviewed like any other pull request, and the reviewer
decides whether it is safe to merge. There is no separate class of files and
no separate list of approvers — this repository is co-created, and the people
who work in it are the ones who decide what goes into it.

That also means the validator itself is not fenced off: a pull request that
changes the validator changes what "valid" means for everybody. The reviewer
is asked to look at it with that in mind.
