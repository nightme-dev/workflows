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

## The site

The repository has an index: <https://nightme-dev.github.io/workflows/>. It
is published by the CI on every push to `main` that touches a workflow or
the site itself, so the index never has to be kept by hand. The CI puts the
built site on the `gh-pages` branch, and that branch is what GitHub Pages
serves.

The site holds:

- the home page, with every workflow and the namespace it belongs to;
- a page per namespace, with the workflows of that namespace;
- a page per workflow, with its title, its arguments, its triggers, its
  workspaces, its jobs and steps, the actions it ships and the commit that
  changed it last;
- the changes page, with the commits that touched the workflows and what
  they did;
- a search box on every page, which searches the whole site;
- the machine-readable files of the API, described below.

The site is built from the workflows and from the templates of the
[`templates/`](templates/) directory. The templates are the only source of
the layout: a change of the layout is a change of the templates and of
nothing else. The generator is [`cmd/site`](cmd/site), and it reads the
workflows with the same parser the validator uses.

The site source and the built site are not in this branch. To see the
site locally, generate the source and serve it:

```sh
go run ./cmd/site --templates templates --workflows workflows --out docs
bundle exec jekyll serve --source docs --destination .site
```

The first command writes `docs/`: the facts of the workflows, the pages, and
a copy of the templates. The second one builds the site and serves it at
<http://localhost:4000/workflows/>. The search needs the index of Pagefind,
which is built after Jekyll:

```sh
pagefind --site .site
```

### The API

The site serves a machine-readable API as well, so that another program can
list the workflows and describe them without reading the HTML. The files
are written by the same generator that writes the site, into the site
source, and Jekyll copies them into the built site as they are:

- `api/index.json` lists every workflow of the repository, in the order of
  its addresses: its address, its identity, its title, its summary, its
  arguments, its triggers, its workspaces, the links to its page, to its
  own file of the API and to its file in the repository, and the commit that
  changed it last;
- `api/<ns>/<name>.json` holds the whole record of one workflow: the fields
  of the index plus its jobs, their steps and the actions it ships.

The index carries the version of its shape, the number of workflows it
lists, the namespaces of those workflows, and the date of the newest commit
that touched the workflows. The date comes from the history of the
repository and not from the clock: two builds of the same tree are the same
bytes, so a rebuild of the site that changes nothing publishes nothing.

The links of the API are absolute, and the base they are built from is the
`--site-url` flag of the generator:

```sh
go run ./cmd/site --templates templates --workflows workflows --out docs \
  --site-url https://nightme-dev.github.io/workflows
```

The flag defaults to that URL, which is where the site is published.
