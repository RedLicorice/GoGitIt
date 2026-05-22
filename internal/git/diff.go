package git

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	fdiff "github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	gitdiff "github.com/go-git/go-git/v5/utils/diff"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// diffContextLines is how many unchanged lines we keep around each change,
// matching git's default unified-diff context.
const diffContextLines = 3

// DiffLine is a single line within a hunk. A line is present on the old side,
// the new side, or both; the absent side's number is 0.
type DiffLine struct {
	Type    string `json:"type"` // context | add | delete
	Content string `json:"content"`
	OldLine int    `json:"old_line"`
	NewLine int    `json:"new_line"`
}

// DiffHunk is a contiguous block of changes plus its surrounding context.
type DiffHunk struct {
	OldStart int        `json:"old_start"`
	OldLines int        `json:"old_lines"`
	NewStart int        `json:"new_start"`
	NewLines int        `json:"new_lines"`
	Header   string     `json:"header"`
	Lines    []DiffLine `json:"lines"`
}

// FileDiff is the structured diff of a single file, ready for the frontend to
// render either inline or side-by-side.
type FileDiff struct {
	Path      string     `json:"path"`
	OldPath   string     `json:"old_path"` // set only for renames
	Status    string     `json:"status"`   // A | M | D | R
	Binary    bool       `json:"binary"`
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
	Hunks     []DiffHunk `json:"hunks"`
}

// rawOp is a normalized diff operation shared by the go-git patch path and the
// worktree (diffmatchpatch) path.
type rawOp int

const (
	opEqual rawOp = iota
	opAdd
	opDelete
)

type rawChunk struct {
	op   rawOp
	text string
}

// CommitDiff returns the structured diff for every file touched by a commit,
// compared against its first parent (or the empty tree for a root commit).
func CommitDiff(repo *gogit.Repository, hash string) ([]FileDiff, error) {
	commit, err := repo.CommitObject(plumbing.NewHash(hash))
	if err != nil {
		return nil, fmt.Errorf("commit %s: %w", hash, err)
	}

	commitTree, err := commit.Tree()
	if err != nil {
		return nil, fmt.Errorf("commit tree: %w", err)
	}

	var parentTree *object.Tree
	if commit.NumParents() > 0 {
		parent, err := commit.Parent(0)
		if err != nil {
			return nil, fmt.Errorf("parent: %w", err)
		}
		parentTree, err = parent.Tree()
		if err != nil {
			return nil, fmt.Errorf("parent tree: %w", err)
		}
	}

	// object.DiffTree treats a nil tree as empty, so root commits work.
	changes, err := object.DiffTree(parentTree, commitTree)
	if err != nil {
		return nil, fmt.Errorf("diff tree: %w", err)
	}
	patch, err := changes.Patch()
	if err != nil {
		return nil, fmt.Errorf("patch: %w", err)
	}

	return filePatchesToDiffs(patch.FilePatches()), nil
}

// WorktreeDiff returns the structured diff of a single file in the working
// tree. When staged is true it diffs HEAD against the index (what a commit
// would record); otherwise it diffs the index — or HEAD, for untracked
// files — against the file currently on disk.
func WorktreeDiff(repo *gogit.Repository, path string, staged bool) (*FileDiff, error) {
	headText, headOK, err := headFileContent(repo, path)
	if err != nil {
		return nil, err
	}
	indexText, indexOK, err := indexFileContent(repo, path)
	if err != nil {
		return nil, err
	}
	wtText, wtOK, err := worktreeFileContent(repo, path)
	if err != nil {
		return nil, err
	}

	var oldText, newText string
	var oldExists, newExists bool
	if staged {
		oldText, oldExists = headText, headOK
		newText, newExists = indexText, indexOK
	} else {
		if indexOK {
			oldText, oldExists = indexText, true
		} else {
			oldText, oldExists = headText, headOK
		}
		newText, newExists = wtText, wtOK
	}

	fd := &FileDiff{Path: path, Hunks: []DiffHunk{}}
	switch {
	case !oldExists && newExists:
		fd.Status = "A"
	case oldExists && !newExists:
		fd.Status = "D"
	default:
		fd.Status = "M"
	}

	if isBinary(oldText) || isBinary(newText) {
		fd.Binary = true
		return fd, nil
	}

	raw := make([]rawChunk, 0)
	for _, d := range gitdiff.Do(oldText, newText) {
		switch d.Type {
		case diffmatchpatch.DiffInsert:
			raw = append(raw, rawChunk{op: opAdd, text: d.Text})
		case diffmatchpatch.DiffDelete:
			raw = append(raw, rawChunk{op: opDelete, text: d.Text})
		default:
			raw = append(raw, rawChunk{op: opEqual, text: d.Text})
		}
	}
	fd.Hunks = buildHunks(flattenChunks(raw))
	fd.Additions, fd.Deletions = countChanges(fd.Hunks)
	return fd, nil
}

// filePatchesToDiffs converts go-git file patches into FileDiff values.
func filePatchesToDiffs(fps []fdiff.FilePatch) []FileDiff {
	out := make([]FileDiff, 0, len(fps))
	for _, fp := range fps {
		from, to := fp.Files()
		fd := FileDiff{Hunks: []DiffHunk{}}

		switch {
		case from == nil && to != nil:
			fd.Status, fd.Path = "A", to.Path()
		case from != nil && to == nil:
			fd.Status, fd.Path = "D", from.Path()
		case from != nil && to != nil:
			fd.Path = to.Path()
			if from.Path() != to.Path() {
				fd.Status, fd.OldPath = "R", from.Path()
			} else {
				fd.Status = "M"
			}
		default:
			continue
		}

		if fp.IsBinary() {
			fd.Binary = true
			out = append(out, fd)
			continue
		}

		raw := make([]rawChunk, 0, len(fp.Chunks()))
		for _, ch := range fp.Chunks() {
			switch ch.Type() {
			case fdiff.Add:
				raw = append(raw, rawChunk{op: opAdd, text: ch.Content()})
			case fdiff.Delete:
				raw = append(raw, rawChunk{op: opDelete, text: ch.Content()})
			default:
				raw = append(raw, rawChunk{op: opEqual, text: ch.Content()})
			}
		}
		fd.Hunks = buildHunks(flattenChunks(raw))
		fd.Additions, fd.Deletions = countChanges(fd.Hunks)
		out = append(out, fd)
	}
	return out
}

// flattenChunks expands operation-tagged blocks into a numbered list of lines.
func flattenChunks(chunks []rawChunk) []DiffLine {
	var lines []DiffLine
	oldNum, newNum := 1, 1
	for _, ch := range chunks {
		for _, content := range splitLines(ch.text) {
			switch ch.op {
			case opEqual:
				lines = append(lines, DiffLine{Type: "context", Content: content, OldLine: oldNum, NewLine: newNum})
				oldNum++
				newNum++
			case opAdd:
				lines = append(lines, DiffLine{Type: "add", Content: content, NewLine: newNum})
				newNum++
			case opDelete:
				lines = append(lines, DiffLine{Type: "delete", Content: content, OldLine: oldNum})
				oldNum++
			}
		}
	}
	return lines
}

// buildHunks groups changed lines into hunks, cropping runs of context longer
// than 2*diffContextLines so unchanged regions collapse like a real diff.
func buildHunks(lines []DiffLine) []DiffHunk {
	n := len(lines)
	keep := make([]bool, n)
	for i := range lines {
		if lines[i].Type == "context" {
			continue
		}
		lo, hi := i-diffContextLines, i+diffContextLines
		if lo < 0 {
			lo = 0
		}
		if hi > n-1 {
			hi = n - 1
		}
		for j := lo; j <= hi; j++ {
			keep[j] = true
		}
	}

	hunks := []DiffHunk{}
	for i := 0; i < n; {
		if !keep[i] {
			i++
			continue
		}
		h := DiffHunk{}
		j := i
		for j < n && keep[j] {
			ln := lines[j]
			if ln.OldLine != 0 {
				if h.OldStart == 0 {
					h.OldStart = ln.OldLine
				}
				h.OldLines++
			}
			if ln.NewLine != 0 {
				if h.NewStart == 0 {
					h.NewStart = ln.NewLine
				}
				h.NewLines++
			}
			h.Lines = append(h.Lines, ln)
			j++
		}
		h.Header = fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
		hunks = append(hunks, h)
		i = j
	}
	return hunks
}

func countChanges(hunks []DiffHunk) (adds, dels int) {
	for _, h := range hunks {
		for _, l := range h.Lines {
			switch l.Type {
			case "add":
				adds++
			case "delete":
				dels++
			}
		}
	}
	return adds, dels
}

// splitLines splits text into lines, dropping the trailing empty element that
// a final newline produces.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "\n")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// isBinary applies git's loose heuristic: a NUL byte near the start of content.
func isBinary(s string) bool {
	n := len(s)
	if n > 8000 {
		n = 8000
	}
	return strings.IndexByte(s[:n], 0) >= 0
}

func headFileContent(repo *gogit.Repository, path string) (string, bool, error) {
	head, err := repo.Head()
	if err != nil {
		// No HEAD (unborn branch / empty repo) — treat the file as absent.
		return "", false, nil
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return "", false, fmt.Errorf("head commit: %w", err)
	}
	f, err := commit.File(path)
	if err != nil {
		if errors.Is(err, object.ErrFileNotFound) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("head file: %w", err)
	}
	content, err := f.Contents()
	if err != nil {
		return "", false, fmt.Errorf("head contents: %w", err)
	}
	return content, true, nil
}

func indexFileContent(repo *gogit.Repository, path string) (string, bool, error) {
	idx, err := repo.Storer.Index()
	if err != nil {
		return "", false, fmt.Errorf("index: %w", err)
	}
	entry, err := idx.Entry(path)
	if err != nil {
		if errors.Is(err, index.ErrEntryNotFound) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("index entry: %w", err)
	}
	blob, err := repo.BlobObject(entry.Hash)
	if err != nil {
		return "", false, fmt.Errorf("index blob: %w", err)
	}
	r, err := blob.Reader()
	if err != nil {
		return "", false, fmt.Errorf("blob reader: %w", err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return "", false, fmt.Errorf("blob read: %w", err)
	}
	return string(data), true, nil
}

func worktreeFileContent(repo *gogit.Repository, path string) (string, bool, error) {
	wt, err := repo.Worktree()
	if err != nil {
		return "", false, fmt.Errorf("worktree: %w", err)
	}
	f, err := wt.Filesystem.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("worktree open: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return "", false, fmt.Errorf("worktree read: %w", err)
	}
	return string(data), true, nil
}
