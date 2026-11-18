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

## Workflows

| Address | What it does |
|---|---|
| `nightme/coder` | Pick an open issue from the nightme/assigned queue, fix it in a worktree, review the change, and open a pull request. |

The catalog above is written by hand and checked against the tree by the
validator: a workflow that is not listed here is reported, and so is an entry
that names no workflow.

## Validating a workflow

The validator is the machine half of the community process. It checks the
objective facts — the file parses, it is a workflow, the names are shaped
right, every action is wired, the catalog agrees with the tree — and leaves
the judgment to the reviewer.

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

The rules that need the whole tree — the shape of the names, the uniqueness of
the identity, the catalog — need the whole tree: they are checked when the
validator is pointed at the workflows directory.

The rules a workflow has to follow, the process a pull request goes through,
and the questions a reviewer answers are in [CONTRIBUTING.md](CONTRIBUTING.md).
