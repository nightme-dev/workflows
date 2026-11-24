package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// repoURL is where the repository lives. The links of the site point
// there: the source of a workflow and the commit that changed it.
const repoURL = "https://github.com/nightme-dev/workflows"

// rawRepoURL is where the files of the repository live as they are
// written, without the interface of GitHub around them.
const rawRepoURL = "https://raw.githubusercontent.com/nightme-dev/workflows"

// blobURL returns the URL of a file of the repository.
func blobURL(path string) string {
	return repoURL + "/blob/main/" + path
}

// rawURL returns the URL of a raw file of the repository.
//
// The raw file is the file itself: a consumer that wants the workflow
// as it is written fetches it from there.
func rawURL(path string) string {
	return rawRepoURL + "/main/" + path
}

// commitURL returns the URL of a commit of the repository.
func commitURL(sha string) string {
	return repoURL + "/commit/" + sha
}

// emptyTree is the tree of nothing: git knows it, so a diff against
// it shows every file of a commit that has no parent.
const emptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// history tells the generator what the git history says.
//
// It is an interface so a test can say what the history holds without
// a repository: the site is built from the history, and a history
// that says nothing is a history of a repository that has no
// commits.
type history interface {
	// lastCommit returns the commit that last changed the bundle at
	// the path, and whether the history knows one. The path is
	// relative to the workflows directory.
	lastCommit(relPath string) (commitRecord, bool)
	// changes returns the commits that touched the workflows, the
	// newest first, at most limit of them.
	changes(limit int) ([]changeRecord, error)
	// lastChange returns the date of the newest commit that touched
	// the workflows, and whether the history knows one.
	//
	// The date is read by a query of its own: the changes page may
	// hold no commit at all, and the index of the API still says when
	// the workflows last changed.
	lastChange() (string, bool)
}

// gitHistory is the history of a git repository.
type gitHistory struct {
	// root is the root of the repository.
	root string
	// workflows is the path of the workflows directory relative to
	// the root of the repository.
	workflows string
}

// openHistory opens the history of the repository the workflows live
// in.
//
// The history has to be the whole one: the site says which commit
// changed a workflow last, and a shallow clone does not know it.
func openHistory(dir string) (*gitHistory, error) {
	root, err := gitRoot(dir)
	if err != nil {
		return nil, err
	}
	shallow, err := gitShallow(root)
	if err != nil {
		return nil, err
	}
	if shallow {
		return nil, &unreadableError{reason: "the git history is not complete: a shallow clone does not say which commit changed a workflow last; fetch the whole history (fetch-depth: 0)"}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, &unreadableError{reason: fmt.Sprintf("%s: %v", dir, err)}
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return nil, &unreadableError{reason: fmt.Sprintf("%s: %v", dir, err)}
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, &unreadableError{reason: fmt.Sprintf("%s: the workflows directory is not inside the repository %s", dir, root)}
	}
	return &gitHistory{root: root, workflows: filepath.ToSlash(rel)}, nil
}

// lastCommit returns the commit that last changed the bundle at the
// path.
func (h *gitHistory) lastCommit(relPath string) (commitRecord, bool) {
	path := filepath.ToSlash(filepath.Join(h.workflows, relPath))
	out, err := runGit(h.root, "log", "-1", "--format=%h%x09%ad%x09%s", "--date=short", "--", path)
	if err != nil {
		return commitRecord{}, false
	}
	line := strings.TrimSpace(out)
	if line == "" {
		return commitRecord{}, false
	}
	parts := strings.SplitN(line, "\t", 3)
	if len(parts) < 3 {
		return commitRecord{}, false
	}
	return commitRecord{
		SHA:     parts[0],
		Date:    parts[1],
		Subject: parts[2],
		URL:     commitURL(parts[0]),
	}, true
}

// lastChange returns the date of the newest commit that touched the
// workflows.
//
// The date is the one of the commit itself: two builds of the same
// tree say the same thing, so the build of the site stays
// reproducible.
func (h *gitHistory) lastChange() (string, bool) {
	out, err := runGit(h.root, "log", "-1", "--format=%ad", "--date=short", "--", h.workflows)
	if err != nil {
		return "", false
	}
	date := strings.TrimSpace(out)
	if date == "" {
		return "", false
	}
	return date, true
}

// changes returns the commits that touched the workflows, the newest
// first.
//
// Every commit says what it did to the workflows: which addresses it
// added, which it changed and which it took away.
func (h *gitHistory) changes(limit int) ([]changeRecord, error) {
	out := []changeRecord{}
	if limit <= 0 {
		return out, nil
	}
	log, err := runGit(h.root, "log", "-n", strconv.Itoa(limit),
		"--format=%H%x09%h%x09%ad%x09%P%x09%s", "--date=short", "--", h.workflows)
	if err != nil {
		return nil, &unreadableError{reason: fmt.Sprintf("the git history cannot be read: %v", err)}
	}
	for _, line := range strings.Split(strings.TrimSpace(log), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 5)
		if len(parts) < 5 {
			continue
		}
		sha, short, date, parents, subject := parts[0], parts[1], parts[2], parts[3], parts[4]
		base := parentOf(parents)
		// The paths of a diff are quoted when they hold characters
		// outside the ASCII: the quote would hide the path.
		diff, err := runGit(h.root, "-c", "core.quotePath=false",
			"diff", "--name-status", "--no-renames", base, sha, "--", h.workflows)
		if err != nil {
			return nil, &unreadableError{reason: fmt.Sprintf("the changes of %s cannot be read: %v", short, err)}
		}
		added, modified, removed := classify(parseNameStatus(diff), h.workflows)
		out = append(out, changeRecord{
			SHA:      short,
			Date:     date,
			Subject:  subject,
			URL:      commitURL(short),
			Added:    added,
			Modified: modified,
			Removed:  removed,
		})
	}
	return out, nil
}

// parentOf returns the commit a commit grew out of.
//
// The parents are the ones the history names: the first of them is
// the one the commit grew out of. A commit that has no parent grew
// out of nothing: the diff against the empty tree shows everything it
// holds.
func parentOf(parents string) string {
	fields := strings.Fields(parents)
	if len(fields) == 0 {
		return emptyTree
	}
	return fields[0]
}

// fileChange is one path a commit changed.
type fileChange struct {
	// status is what happened to the path: "A" for a file that was
	// added, "D" for one that was taken away, "M" for one that was
	// changed.
	status string
	// path is the path of the file, relative to the root of the
	// repository.
	path string
}

// parseNameStatus reads the output of git diff --name-status.
func parseNameStatus(diff string) []fileChange {
	var out []fileChange
	for _, line := range strings.Split(strings.TrimSpace(diff), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 2 {
			continue
		}
		out = append(out, fileChange{status: strings.TrimSpace(parts[0]), path: strings.TrimSpace(parts[1])})
	}
	return out
}

// classify says which addresses a commit added, changed and took
// away.
//
// The address of a bundle is decided by its workflow file: the file
// that was added is a bundle that came, the file that was taken away
// is a bundle that went. A bundle whose workflow file was not touched
// but whose other files were has been changed.
func classify(changes []fileChange, workflows string) (added, modified, removed []string) {
	added = []string{}
	modified = []string{}
	removed = []string{}
	files := map[string]map[string]string{}
	for _, change := range changes {
		address, ok := addressOf(change.path, workflows)
		if !ok {
			continue
		}
		paths := files[address]
		if paths == nil {
			paths = map[string]string{}
			files[address] = paths
		}
		paths[change.path] = change.status
	}
	for address, paths := range files {
		switch statusOf(paths, address, workflows) {
		case "A":
			added = append(added, address)
		case "D":
			removed = append(removed, address)
		default:
			modified = append(modified, address)
		}
	}
	sort.Strings(added)
	sort.Strings(modified)
	sort.Strings(removed)
	return added, modified, removed
}

// statusOf says what a commit did to a bundle.
func statusOf(paths map[string]string, address, workflows string) string {
	status, ok := paths[workflows+"/"+address+"/"+workflowFile]
	if ok {
		switch status {
		case "A":
			return "A"
		case "D":
			return "D"
		}
		return "M"
	}
	all := "A"
	for _, status := range paths {
		if status != "A" {
			all = "M"
		}
	}
	if all == "M" {
		return "M"
	}
	all = "D"
	for _, status := range paths {
		if status != "D" {
			all = "M"
		}
	}
	return all
}

// addressOf returns the address of the bundle a path belongs to.
//
// A path that is not inside a bundle belongs to none.
func addressOf(path, workflows string) (string, bool) {
	prefix := workflows + "/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	rest := strings.Split(strings.TrimPrefix(path, prefix), "/")
	if len(rest) < 2 {
		return "", false
	}
	return rest[0] + "/" + rest[1], true
}

// gitRoot returns the root of the repository a directory lives in.
func gitRoot(dir string) (string, error) {
	out, err := runGit(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", &unreadableError{reason: fmt.Sprintf("the git history cannot be read: %v", err)}
	}
	return strings.TrimSpace(out), nil
}

// gitShallow says whether the history of a repository is a shallow
// one.
func gitShallow(root string) (bool, error) {
	out, err := runGit(root, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return false, &unreadableError{reason: fmt.Sprintf("the git history cannot be read: %v", err)}
	}
	return strings.TrimSpace(out) == "true", nil
}

// runGit runs a git command and returns what it printed.
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), message)
	}
	return stdout.String(), nil
}
