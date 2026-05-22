package git

import (
	"fmt"

	"github.com/RedLicorice/GoGitIt/internal/gitext"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

// Index- and worktree-mutating operations. These shell out to the system git
// binary via internal/gitext: go-git's worktree/index code mishandles
// core.fileMode, .gitignore and untracked files and is not atomic. go-git is
// kept for object-database reads (log, diff, graph) where it is reliable.

// Stage adds the given paths to the index.
func Stage(repo *gogit.Repository, paths []string) error {
	return gitext.Stage(worktreeRoot(repo), paths)
}

// Unstage resets the given paths in the index back to HEAD, keeping the working
// tree — the equivalent of `git restore --staged`.
func Unstage(repo *gogit.Repository, paths []string) error {
	return gitext.Unstage(worktreeRoot(repo), paths)
}

// Discard reverts the given paths to their HEAD state, dropping both staged and
// unstaged changes. Paths that are untracked are deleted from the working tree.
// This is irreversible.
func Discard(repo *gogit.Repository, paths []string) error {
	root := worktreeRoot(repo)
	st, err := gitext.Status(root)
	if err != nil {
		return err
	}
	untracked := map[string]bool{}
	for _, u := range st.Untracked {
		untracked[u] = true
	}
	// Tracked paths are restored to HEAD; untracked ones are cleaned away.
	var restore, clean []string
	for _, p := range paths {
		if untracked[p] {
			clean = append(clean, p)
		} else {
			restore = append(restore, p)
		}
	}
	if err := gitext.Restore(root, restore); err != nil {
		return err
	}
	return gitext.Clean(root, clean)
}

// CommitResult identifies a newly created commit.
type CommitResult struct {
	Hash      string `json:"hash"`
	ShortHash string `json:"short_hash"`
}

// CreateCommit records the staged index as a new commit. name and email are
// the preferred author identity (from app settings); when either is blank the
// repo's / global git config is consulted as a fallback.
func CreateCommit(repo *gogit.Repository, message, name, email string) (*CommitResult, error) {
	if name == "" || email == "" {
		if cfg, err := repo.ConfigScoped(config.LocalScope); err == nil {
			if name == "" {
				name = cfg.User.Name
			}
			if email == "" {
				email = cfg.User.Email
			}
		}
	}
	if name == "" || email == "" {
		return nil, fmt.Errorf("no commit identity — set your name and email in Settings")
	}
	hash, err := gitext.Commit(worktreeRoot(repo), message, name, email)
	if err != nil {
		return nil, err
	}
	short := hash
	if len(hash) >= 7 {
		short = hash[:7]
	}
	return &CommitResult{Hash: hash, ShortHash: short}, nil
}
