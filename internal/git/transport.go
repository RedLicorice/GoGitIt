package git

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/RedLicorice/GoGitIt/internal/gitext"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

// ResolveAuth picks a go-git auth method for a remote URL:
//
//   - SSH URL   → ssh-agent if running, else a default unencrypted ~/.ssh key.
//   - HTTPS URL → the GitHub CLI token for github.com hosts, otherwise the
//     stored PAT; both sent as HTTP basic auth (token as password).
//
// A nil AuthMethod (no error) means anonymous — fine for fetching public
// repos, but a push will fail.
func ResolveAuth(remoteURL, ghToken, pat string) (transport.AuthMethod, error) {
	if !isHTTPS(remoteURL) {
		return sshAuth()
	}

	token := pat
	if host := httpsHost(remoteURL); host == "github.com" && ghToken != "" {
		token = ghToken // prefer the gh login for GitHub
	}
	if token == "" {
		token = ghToken // last resort
	}
	if token == "" {
		return nil, nil
	}
	return &githttp.BasicAuth{Username: "git", Password: token}, nil
}

// Fetch updates remote-tracking refs from the named remote.
func Fetch(repo *gogit.Repository, remoteName string, auth transport.AuthMethod) error {
	err := repo.Fetch(&gogit.FetchOptions{RemoteName: remoteName, Auth: auth})
	if err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return fmt.Errorf("fetch: %w", err)
	}
	return nil
}

// DivergenceInfo is returned by Pull when the branch has diverged from its
// upstream — both ahead and behind. It is not an error; the caller should
// surface a confirmation dialog offering Rebase & Push as the resolution path.
type DivergenceInfo struct {
	Ahead         int
	Behind        int
	LocalCommits  []string // git log --oneline upstream..HEAD
	RemoteCommits []string // git log --oneline HEAD..upstream
	Dirty         bool     // working tree has uncommitted changes
}

// Pull fetches and fast-forwards the current branch, or returns a non-nil
// DivergenceInfo when the branch has diverged (both ahead and behind — not
// an error). A nil DivergenceInfo + nil error means success.
func Pull(repo *gogit.Repository, remoteName string, auth transport.AuthMethod) (*DivergenceInfo, error) {
	head, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("head: %w", err)
	}
	if head.Name() == plumbing.HEAD {
		return nil, fmt.Errorf("cannot pull onto a detached HEAD — check out a branch first")
	}
	if err := Fetch(repo, remoteName, auth); err != nil {
		return nil, err
	}
	branch := head.Name().Short()
	upstream := remoteName + "/" + branch
	if _, err := repo.Reference(plumbing.NewRemoteReferenceName(remoteName, branch), true); err != nil {
		return nil, fmt.Errorf("no upstream %s — nothing to pull", upstream)
	}
	root := worktreeRoot(repo)
	ahead, behind, err := gitext.AheadBehind(root, upstream)
	if err != nil {
		return nil, err
	}
	if ahead > 0 && behind > 0 {
		localCommits, _ := gitext.LogOneline(root, upstream+"..HEAD")
		remoteCommits, _ := gitext.LogOneline(root, "HEAD.."+upstream)
		dirty, _ := gitext.IsDirty(root)
		return &DivergenceInfo{
			Ahead:         ahead,
			Behind:        behind,
			LocalCommits:  localCommits,
			RemoteCommits: remoteCommits,
			Dirty:         dirty,
		}, nil
	}
	if behind > 0 {
		if err := gitext.MergeFF(root, upstream); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// RebasePush resolves a diverged branch: stashes if dirty, rebases onto the
// upstream, pushes, then pops the stash. On rebase conflict the rebase is
// aborted and an error describing the conflicting files is returned.
func RebasePush(repo *gogit.Repository, remoteName string, auth transport.AuthMethod) (string, error) {
	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("head: %w", err)
	}
	branch := head.Name().Short()
	upstream := remoteName + "/" + branch
	root := worktreeRoot(repo)

	dirty, err := gitext.IsDirty(root)
	if err != nil {
		return "", err
	}
	stashed := false
	if dirty {
		if err := gitext.Stash(root); err != nil {
			return "", fmt.Errorf("stash: %w", err)
		}
		stashed = true
	}

	conflictFiles, rebaseErr := gitext.RebaseOnto(root, upstream)
	if rebaseErr != nil {
		if stashed {
			_ = gitext.StashPop(root)
		}
		if len(conflictFiles) > 0 {
			return "", fmt.Errorf("rebase conflict in: %s", strings.Join(conflictFiles, ", "))
		}
		return "", fmt.Errorf("rebase failed: %w", rebaseErr)
	}

	if err := Push(repo, remoteName, auth); err != nil {
		return "", err
	}

	if stashed {
		if err := gitext.StashPop(root); err != nil {
			return "", fmt.Errorf("push succeeded but stash pop conflicted — resolve manually with 'git stash pop': %w", err)
		}
	}

	return remoteName, nil
}

// Push sends the current branch to the named remote and advances the local
// remote-tracking ref so push state reflects reality without a follow-up fetch.
func Push(repo *gogit.Repository, remoteName string, auth transport.AuthMethod) error {
	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("head: %w", err)
	}
	if head.Name() == plumbing.HEAD {
		return fmt.Errorf("cannot push a detached HEAD — check out a branch first")
	}
	branch := head.Name().Short()
	refspec := config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", branch, branch))

	err = repo.Push(&gogit.PushOptions{
		RemoteName: remoteName,
		RefSpecs:   []config.RefSpec{refspec},
		Auth:       auth,
	})
	if err != nil && !errors.Is(err, gogit.NoErrAlreadyUpToDate) {
		return fmt.Errorf("push: %w", err)
	}

	tracking := plumbing.NewRemoteReferenceName(remoteName, branch)
	_ = repo.Storer.SetReference(plumbing.NewHashReference(tracking, head.Hash()))
	return nil
}

// CurrentRemote returns the remote the checked-out branch tracks, or "origin".
func CurrentRemote(repo *gogit.Repository) string {
	if head, err := repo.Head(); err == nil && head.Name() != plumbing.HEAD {
		branch := head.Name().Short()
		if cfg, err := repo.Config(); err == nil {
			if b, ok := cfg.Branches[branch]; ok && b.Remote != "" {
				return b.Remote
			}
		}
	}
	return "origin"
}

func isHTTPS(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

func httpsHost(u string) string {
	if parsed, err := url.Parse(u); err == nil {
		return parsed.Hostname()
	}
	return ""
}

// sshAuth prefers ssh-agent (which can unlock passphrase-protected keys),
// falling back to a default unencrypted private key in ~/.ssh.
func sshAuth() (transport.AuthMethod, error) {
	if os.Getenv("SSH_AUTH_SOCK") != "" {
		if a, err := gitssh.NewSSHAgentAuth("git"); err == nil {
			return a, nil
		}
	}
	home, err := os.UserHomeDir()
	if err == nil {
		for _, name := range []string{"id_ed25519", "id_rsa", "id_ecdsa"} {
			p := filepath.Join(home, ".ssh", name)
			if _, statErr := os.Stat(p); statErr == nil {
				if a, keyErr := gitssh.NewPublicKeysFromFile("git", p, ""); keyErr == nil {
					return a, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("no SSH credentials — start ssh-agent or add an unencrypted key under ~/.ssh")
}
