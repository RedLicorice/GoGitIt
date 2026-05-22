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

// Pull fetches the named remote and fast-forwards the current branch to its
// upstream. The fetch is go-git (auth-aware, touches no worktree file); the
// fast-forward runs through system git — go-git's Worktree.Pull is not atomic,
// on a dirty worktree it moves the branch ref but leaves the index and
// worktree stale. A non-fast-forward situation errors (no real merge yet).
func Pull(repo *gogit.Repository, remoteName string, auth transport.AuthMethod) error {
	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("head: %w", err)
	}
	if head.Name() == plumbing.HEAD {
		return fmt.Errorf("cannot pull onto a detached HEAD — check out a branch first")
	}
	if err := Fetch(repo, remoteName, auth); err != nil {
		return err
	}
	branch := head.Name().Short()
	upstream := remoteName + "/" + branch
	if _, err := repo.Reference(plumbing.NewRemoteReferenceName(remoteName, branch), true); err != nil {
		return fmt.Errorf("no upstream %s — nothing to pull", upstream)
	}
	if err := gitext.MergeFF(worktreeRoot(repo), upstream); err != nil {
		return err
	}
	return nil
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
