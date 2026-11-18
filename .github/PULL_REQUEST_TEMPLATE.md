## What this workflow does

<!-- What it is for, in a sentence or two. -->

## The namespace

<!-- The GitHub account name your workflow is published under. -->

- [ ] The namespace is my own account, and I am the author of this pull
      request.

## The reviewer's questions

These are the questions the reviewer answers before merging. Answer them here
if you can — the reviewer will check them anyway.

- [ ] **Is the name apt?** Does `<name>` say what the workflow does?
- [ ] **What will the `shell:` steps execute?** List the commands, or say
      that the workflow has none.
- [ ] **What will they write or delete?** Name the files, the branches, the
      pull requests — anything the workflow touches.
- [ ] **Should this be several workflows?** Is one bundle doing the job of
      two?

## The checklist

- [ ] `go run ./cmd/validate ./workflows` exits 0.
- [ ] The address of the workflow is listed in the `## Workflows` section of
      the README.
- [ ] The workflow file holds only the keys of the schema.
- [ ] Every script of the `actions/` directory is executable and is used by a
      step.
