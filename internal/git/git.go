package git

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/RedLicorice/GoGitIt/internal/gitext"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Status describes the working tree state at a glance.
type Status struct {
	Branch string `json:"branch"`
	Detached bool `json:"detached"`
	// LocalBranch is true when the repo has a remote but the current branch
	// has no tracking ref (created locally, never pushed).
	LocalBranch bool         `json:"local_branch"`
	Ahead       int          `json:"ahead"`
	Behind      int          `json:"behind"`
	Staged    []FileChange `json:"staged"`
	Unstaged  []FileChange `json:"unstaged"`
	Untracked []string     `json:"untracked"`
}

type FileChange struct {
	Path   string `json:"path"`
	Status string `json:"status"` // M, A, D, R, ?
}

type Commit struct {
	Hash      string    `json:"hash"`
	ShortHash string    `json:"short_hash"`
	Author    string    `json:"author"`
	Email     string    `json:"email"`
	Date      time.Time `json:"date"`
	Message   string    `json:"message"`
	Parents   []string  `json:"parents"`
	// Pushed is false when the commit has not reached the branch's remote. It
	// stays true only when the repo has no remote at all (push state is N/A).
	Pushed bool `json:"pushed"`
}

type Branch struct {
	Name      string `json:"name"`
	IsCurrent bool   `json:"is_current"`
	IsRemote  bool   `json:"is_remote"`
	Hash      string `json:"hash"`
}

// Open opens an existing repository at path.
func Open(path string) (*gogit.Repository, error) {
	return gogit.PlainOpen(path)
}

// GetStatus returns a structured snapshot of the working tree. The file lists
// come from the system `git` binary (go-git's Worktree.Status mishandles
// core.fileMode and .gitignore); branch / ahead-behind come from go-git. Falls
// back to go-git's Status when the git binary is unavailable.
func GetStatus(repo *gogit.Repository) (*Status, error) {
	s := &Status{
		Branch:    currentBranch(repo),
		Detached:  isDetached(repo),
		Staged:    []FileChange{},
		Unstaged:  []FileChange{},
		Untracked: []string{},
	}
	s.Ahead, s.Behind, s.LocalBranch = aheadBehind(repo)

	// Primary path: `git status` — the only reliable source for the file list.
	if root := worktreeRoot(repo); root != "" {
		if res, err := gitext.Status(root); err == nil {
			for _, f := range res.Staged {
				s.Staged = append(s.Staged, FileChange{Path: f.Path, Status: f.Code})
			}
			for _, f := range res.Unstaged {
				s.Unstaged = append(s.Unstaged, FileChange{Path: f.Path, Status: f.Code})
			}
			s.Untracked = append(s.Untracked, res.Untracked...)
			return s, nil
		}
	}

	// Fallback: go-git's own Status (used only when git is unavailable). It
	// does not honour core.fileMode, so mode-only false positives are dropped.
	wt, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("worktree: %w", err)
	}
	wtStatus, err := wt.Status()
	if err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}
	ignoreMode := fileModeIgnored(repo)

	for path, fs := range wtStatus {
		// Untracked: present in worktree, not in index.
		if fs.Staging == gogit.Untracked && fs.Worktree == gogit.Untracked {
			s.Untracked = append(s.Untracked, path)
			continue
		}
		if fs.Staging != gogit.Unmodified && fs.Staging != gogit.Untracked {
			if !(ignoreMode && fs.Staging == gogit.Modified && contentUnchanged(repo, path, true)) {
				s.Staged = append(s.Staged, FileChange{Path: path, Status: string(fs.Staging)})
			}
		}
		if fs.Worktree != gogit.Unmodified && fs.Worktree != gogit.Untracked {
			if !(ignoreMode && fs.Worktree == gogit.Modified && contentUnchanged(repo, path, false)) {
				s.Unstaged = append(s.Unstaged, FileChange{Path: path, Status: string(fs.Worktree)})
			}
		}
	}

	return s, nil
}

// GetLog returns up to `limit` commits reachable from HEAD.
func GetLog(repo *gogit.Repository, limit int) ([]Commit, error) {
	if limit <= 0 {
		limit = 50
	}

	ref, err := repo.Head()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			// Unborn branch / empty repo — no commits yet.
			return []Commit{}, nil
		}
		return nil, err
	}

	iter, err := repo.Log(&gogit.LogOptions{From: ref.Hash()})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	// Commits reachable from the upstream ref are considered pushed.
	pushedSet := remoteReachableSet(repo, ref)

	commits := make([]Commit, 0, limit)
	count := 0
	err = iter.ForEach(func(c *object.Commit) error {
		if count >= limit {
			return errStopIteration
		}
		parents := make([]string, 0, c.NumParents())
		for _, p := range c.ParentHashes {
			parents = append(parents, p.String())
		}
		pushed := true
		if pushedSet != nil {
			_, pushed = pushedSet[c.Hash]
		}
		commits = append(commits, Commit{
			Hash:      c.Hash.String(),
			ShortHash: c.Hash.String()[:7],
			Author:    c.Author.Name,
			Email:     c.Author.Email,
			Date:      c.Author.When,
			Message:   c.Message,
			Parents:   parents,
			Pushed:    pushed,
		})
		count++
		return nil
	})
	if err != nil && !errors.Is(err, errStopIteration) {
		return nil, err
	}
	return commits, nil
}

// ListBranches returns local and remote branches.
func ListBranches(repo *gogit.Repository) ([]Branch, error) {
	headRef, _ := repo.Head()
	var headName string
	if headRef != nil {
		headName = headRef.Name().Short()
	}

	out := []Branch{}

	locals, err := repo.Branches()
	if err != nil {
		return nil, err
	}
	_ = locals.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().Short()
		out = append(out, Branch{
			Name:      name,
			IsCurrent: name == headName,
			Hash:      ref.Hash().String(),
		})
		return nil
	})

	remotes, err := repo.References()
	if err != nil {
		return nil, err
	}
	_ = remotes.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name().IsRemote() {
			out = append(out, Branch{
				Name:     ref.Name().Short(),
				IsRemote: true,
				Hash:     ref.Hash().String(),
			})
		}
		return nil
	})

	return out, nil
}

// currentBranch returns the short branch name HEAD points at, handling an
// unborn branch (a freshly initialized repo with no commits) where HEAD is a
// symbolic ref to a branch that does not exist yet.
func currentBranch(repo *gogit.Repository) string {
	if head, err := repo.Head(); err == nil {
		return head.Name().Short()
	}
	if ref, err := repo.Reference(plumbing.HEAD, false); err == nil &&
		ref.Type() == plumbing.SymbolicReference {
		return ref.Target().Short()
	}
	return ""
}

// remoteReachableSet returns the set of commit hashes considered "pushed":
//
//   - nil      — the repo has no remote; push state is not meaningful.
//   - empty    — a remote exists but this branch's tracking ref is absent
//     (never pushed / upstream gone): every commit counts as unpushed.
//   - populated — commits reachable from the upstream tracking ref.
func remoteReachableSet(repo *gogit.Repository, headRef *plumbing.Reference) map[plumbing.Hash]struct{} {
	remotes, err := repo.Remotes()
	if err != nil || len(remotes) == 0 {
		return nil // purely local repo — nothing to compare against
	}
	up := upstreamRef(repo, headRef)
	if up == nil {
		// Remote exists but the tracking ref is missing — branch not on the
		// remote yet, so all commits read as unpushed.
		return map[plumbing.Hash]struct{}{}
	}
	return commitSet(repo, up.Hash())
}

// commitSet collects every commit hash reachable from a starting hash.
func commitSet(repo *gogit.Repository, from plumbing.Hash) map[plumbing.Hash]struct{} {
	set := map[plumbing.Hash]struct{}{}
	iter, err := repo.Log(&gogit.LogOptions{From: from})
	if err != nil {
		return set
	}
	defer iter.Close()
	_ = iter.ForEach(func(c *object.Commit) error {
		set[c.Hash] = struct{}{}
		return nil
	})
	return set
}

// aheadBehind counts commits the local branch has that the remote lacks
// (ahead) and commits the remote has that the local branch lacks (behind).
// localOnly is true when a remote exists but this branch has no tracking ref.
// All three are zero/false when the repo has no remote at all.
func aheadBehind(repo *gogit.Repository) (ahead, behind int, localOnly bool) {
	headRef, err := repo.Head()
	if err != nil {
		return 0, 0, false
	}
	pushed := remoteReachableSet(repo, headRef)
	if pushed == nil {
		return 0, 0, false // no remote — not meaningful
	}
	headSet := commitSet(repo, headRef.Hash())
	for h := range headSet {
		if _, ok := pushed[h]; !ok {
			ahead++
		}
	}
	for h := range pushed {
		if _, ok := headSet[h]; !ok {
			behind++
		}
	}
	// An empty pushed-set means the branch has no tracking ref yet.
	localOnly = len(pushed) == 0 && headRef.Name() != plumbing.HEAD
	return ahead, behind, localOnly
}

// upstreamRef resolves the remote-tracking reference for the checked-out
// branch: its configured upstream, falling back to origin/<branch>.
func upstreamRef(repo *gogit.Repository, headRef *plumbing.Reference) *plumbing.Reference {
	if headRef.Name() == plumbing.HEAD {
		return nil // detached HEAD — no branch, no upstream
	}
	branch := headRef.Name().Short()

	if cfg, err := repo.Config(); err == nil {
		if b, ok := cfg.Branches[branch]; ok && b.Remote != "" && b.Merge != "" {
			name := plumbing.NewRemoteReferenceName(b.Remote, b.Merge.Short())
			if ref, err := repo.Reference(name, true); err == nil {
				return ref
			}
		}
	}
	if ref, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", branch), true); err == nil {
		return ref
	}
	return nil
}

// worktreeRoot returns the absolute working-tree path of a repo, or "" if it
// has no worktree (bare).
func worktreeRoot(repo *gogit.Repository) string {
	wt, err := repo.Worktree()
	if err != nil {
		return ""
	}
	return wt.Filesystem.Root()
}

// fileModeIgnored reports whether the repo sets core.fileMode = false, in
// which case git (and so GoGitIt) must ignore executable-bit changes.
func fileModeIgnored(repo *gogit.Repository) bool {
	cfg, err := repo.ConfigScoped(config.LocalScope)
	if err != nil {
		return false
	}
	return strings.EqualFold(cfg.Raw.Section("core").Option("filemode"), "false")
}

// contentUnchanged reports whether a file's content is byte-identical across
// the two sides of a working-tree diff — i.e. go-git flagged it for a mode
// change only. staged compares HEAD↔index; otherwise index↔worktree.
func contentUnchanged(repo *gogit.Repository, path string, staged bool) bool {
	var a, b string
	var aOK, bOK bool
	if staged {
		a, aOK, _ = headFileContent(repo, path)
		b, bOK, _ = indexFileContent(repo, path)
	} else {
		if idx, ok, _ := indexFileContent(repo, path); ok {
			a, aOK = idx, true
		} else {
			a, aOK, _ = headFileContent(repo, path)
		}
		b, bOK, _ = worktreeFileContent(repo, path)
	}
	return aOK && bOK && a == b
}

// isDetached reports whether HEAD points straight at a commit rather than a
// branch. When detached, the resolved HEAD reference is named "HEAD" itself.
func isDetached(repo *gogit.Repository) bool {
	head, err := repo.Head()
	if err != nil {
		return false // unborn branch — not detached
	}
	return head.Name() == plumbing.HEAD
}

var errStopIteration = errors.New("stop")
