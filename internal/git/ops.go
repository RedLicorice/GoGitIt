package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/RedLicorice/GoGitIt/internal/gitext"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

// lfsDefaultThresholdBytes is the auto-track size cutoff when the user hasn't
// configured one — git's own server-side warning threshold is 100 MB.
const lfsDefaultThresholdBytes int64 = 100 * 1024 * 1024

// Index- and worktree-mutating operations. These shell out to the system git
// binary via internal/gitext: go-git's worktree/index code mishandles
// core.fileMode, .gitignore and untracked files and is not atomic. go-git is
// kept for object-database reads (log, diff, graph) where it is reliable.

// Stage adds the given paths to the index. When the repo has LFS
// auto-tracking enabled (gogitit.lfs.enabled), files over the configured
// threshold are git-lfs tracked first so they land as pointers, not blobs.
func Stage(repo *gogit.Repository, paths []string) error {
	root := worktreeRoot(repo)
	if enabled, threshold := lfsAutoConfig(root); enabled && threshold > 0 {
		if lfsTrackOversize(root, paths, threshold) {
			// .gitattributes must be staged so the new pattern is committed.
			paths = append([]string{".gitattributes"}, paths...)
		}
	}
	return gitext.Stage(root, paths)
}

// lfsAutoConfig reads the per-repo LFS auto-tracking settings.
func lfsAutoConfig(root string) (enabled bool, threshold int64) {
	threshold = lfsDefaultThresholdBytes
	v, _ := gitext.ConfigGet(root, "gogitit.lfs.enabled")
	if !strings.EqualFold(v, "true") {
		return false, threshold
	}
	if vt, _ := gitext.ConfigGet(root, "gogitit.lfs.threshold"); vt != "" {
		if n, err := strconv.ParseInt(vt, 10, 64); err == nil && n > 0 {
			threshold = n
		}
	}
	return true, threshold
}

// lfsTrackOversize git-lfs tracks any path larger than threshold. Returns true
// when at least one path was tracked (so .gitattributes must also be staged).
func lfsTrackOversize(root string, paths []string, threshold int64) bool {
	tracked := false
	for _, p := range paths {
		fi, err := os.Stat(filepath.Join(root, p))
		if err != nil || fi.IsDir() || fi.Size() < threshold {
			continue
		}
		if err := gitext.LfsTrack(root, p); err == nil {
			tracked = true
		}
	}
	return tracked
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
