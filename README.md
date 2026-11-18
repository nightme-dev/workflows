# nightme workflows

This repository is the shared place where the nightme community publishes
workflows — the YAML files the personal agent (`/me`) runs. Until now every
workflow was a file somebody hand-copied into `~/.nightme/workflows/`; here a
workflow has an address, a directory of its own, and a pull request that a
reviewer looked at before it was published.

A workflow is a directory under `workflows/`, and its address is its path.

## Layout

```
workflows/
└── <ns>/
    └── <name>/
        ├── workflow.yml     # the workflow itself
        ├── actions/         # optional: the actions the workflow ships
        │   └── <action>.sh
        └── README.md        # optional: notes for humans
```

- **namespace (`<ns>`)** — a GitHub account name. You choose it; the reviewer
  decides whether it legitimately belongs to you.
- **short name (`<name>`)** — lowercase kebab-case.
- **address** — `<ns>/<name>`, e.g. `nightme/coder`. The address is the path,
  so a consumer that knows the address knows exactly which file to fetch.
- **identity** — `<ns>-<name>`, e.g. `nightme-coder`. This is the name the
  workflow carries once it is installed next to other workflows, so two
  workflows in this repository may not share one.

The repository uses the `.yml` extension only.

## Validating a workflow

The validator is the machine half of the community process. It checks the
objective facts — the file parses, it is a workflow, the names are shaped
right, every action is wired — and leaves the judgment to the reviewer.

```sh
go run ./cmd/validate ./workflows
```

It prints nothing when everything is valid. Every problem it finds is one
line, `path:line:column: message`, the format of nightme's own workflow
validator. The exit code is:

| Code | Meaning |
|---|---|
| 0 | everything is valid |
| 8 | a workflow does not validate, or a repository convention is broken |
| 9 | a path does not exist or cannot be read |

Point it at one bundle to check one workflow and the actions it ships:

```sh
go run ./cmd/validate ./workflows/nightme/coder
```

The rules that need the whole tree — the uniqueness of the identity, the
files that sit outside a bundle — need the whole tree: they are checked
when the validator is pointed at the workflows directory.

### The changed bundles

The validator has a second mode: it checks only the bundles it is given. The
values are paths under the workflows directory, and the flag may be given
more than once:

```sh
go run ./cmd/validate --changed nightme/coder ./workflows
go run ./cmd/validate --changed alice/fix,bob/build ./workflows
```

In this mode nothing but the named bundles is read, and the rules that need
the whole tree are not applied: a bundle stands on its own. The shape of the
namespace and the name is checked, the bundle is checked on its own terms,
and a workflow nobody touched cannot fail the run.

CI uses this mode: a pull request is judged on what it changed, so only the
bundles it changed are validated.

The rules a workflow has to follow, the process a pull request goes through,
and the questions a reviewer answers are in [CONTRIBUTING.md](CONTRIBUTING.md).
