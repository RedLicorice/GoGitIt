package git

import (
	"fmt"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// CreateBranch creates a branch at the current commit and switches to it.
// There is deliberately no worktree checkout — the new branch points at the
// same commit, so the tree is unchanged and uncommitted work and untracked
// files are left exactly as they are. (go-git's Checkout would prune untracked
// files here, even though `git checkout -b` never does.)
//
// Switching to an *existing* branch and fast-forward merging genuinely need a
// worktree update; both go through `internal/gitext` (system git) because
// go-git's Checkout/Reset also prune untracked files — see gitext.Checkout.
func CreateBranch(repo *gogit.Repository, name string) error {
	if name == "" {
		return fmt.Errorf("branch name is required")
	}
	ref := plumbing.NewBranchReferenceName(name)
	if _, err := repo.Reference(ref, false); err == nil {
		return fmt.Errorf("branch %q already exists", name)
	}
	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("head: %w", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(ref, head.Hash())); err != nil {
		return fmt.Errorf("create branch %q: %w", name, err)
	}
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, ref)); err != nil {
		return fmt.Errorf("switch HEAD to %q: %w", name, err)
	}
	return nil
}

// CreateBranchAt creates a branch pointing at a specific commit, without
// moving HEAD or touching the worktree. It rescues an abandoned (unreachable)
// commit by making it reachable again.
func CreateBranchAt(repo *gogit.Repository, name, hash string) error {
	if name == "" {
		return fmt.Errorf("branch name is required")
	}
	ref := plumbing.NewBranchReferenceName(name)
	if _, err := repo.Reference(ref, false); err == nil {
		return fmt.Errorf("branch %q already exists", name)
	}
	h := plumbing.NewHash(hash)
	if _, err := repo.CommitObject(h); err != nil {
		return fmt.Errorf("commit %s not found", hash)
	}
	if err := repo.Storer.SetReference(plumbing.NewHashReference(ref, h)); err != nil {
		return fmt.Errorf("create branch %q: %w", name, err)
	}
	return nil
}

// DeleteBranch removes a branch (its ref and any [branch] config). The
// checked-out branch cannot be deleted.
func DeleteBranch(repo *gogit.Repository, name string) error {
	ref := plumbing.NewBranchReferenceName(name)
	if head, err := repo.Head(); err == nil && head.Name() == ref {
		return fmt.Errorf("cannot delete the checked-out branch %q", name)
	}
	if _, err := repo.Reference(ref, false); err != nil {
		return fmt.Errorf("branch %q not found", name)
	}
	if err := repo.Storer.RemoveReference(ref); err != nil {
		return fmt.Errorf("delete branch %q: %w", name, err)
	}
	_ = repo.DeleteBranch(name) // also drop the [branch] config, if any
	return nil
}
