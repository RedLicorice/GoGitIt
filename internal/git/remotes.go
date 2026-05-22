package git

import (
	"fmt"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
)

// Remote is a configured git remote.
type Remote struct {
	Name string   `json:"name"`
	URLs []string `json:"urls"`
}

// ListRemotes returns the repository's configured remotes.
func ListRemotes(repo *gogit.Repository) ([]Remote, error) {
	rs, err := repo.Remotes()
	if err != nil {
		return nil, fmt.Errorf("remotes: %w", err)
	}
	out := make([]Remote, 0, len(rs))
	for _, r := range rs {
		c := r.Config()
		out = append(out, Remote{Name: c.Name, URLs: c.URLs})
	}
	return out, nil
}

// AddRemote registers a new remote.
func AddRemote(repo *gogit.Repository, name, url string) error {
	if name == "" || url == "" {
		return fmt.Errorf("remote name and url are required")
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: name, URLs: []string{url}}); err != nil {
		return fmt.Errorf("create remote %q: %w", name, err)
	}
	return nil
}

// SetRemoteURL replaces the URL of an existing remote, preserving its fetch
// refspecs (go-git has no direct setter, so the config is edited in place).
func SetRemoteURL(repo *gogit.Repository, name, url string) error {
	if url == "" {
		return fmt.Errorf("url is required")
	}
	cfg, err := repo.Config()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	rc, ok := cfg.Remotes[name]
	if !ok {
		return fmt.Errorf("remote %q not found", name)
	}
	rc.URLs = []string{url}
	if err := repo.SetConfig(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// RemoveRemote deletes a remote.
func RemoveRemote(repo *gogit.Repository, name string) error {
	if err := repo.DeleteRemote(name); err != nil {
		return fmt.Errorf("delete remote %q: %w", name, err)
	}
	return nil
}
