// Package settings holds user-controlled application configuration that is
// edited from the settings page — distinct from internal/config, which is the
// operator-set, startup-time configuration.
package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Identity is the author identity stamped on commits created in GoGitIt.
type Identity struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// MCP holds the per-app config for the built-in MCP server (Model Context
// Protocol) so external LLM clients can drive GoGitIt's git operations.
// BasicPass is a secret: stored on disk but never serialised to the client
// (handlers expose only whether one is set).
type MCP struct {
	Enabled   bool   `json:"enabled"`
	AuthMode  string `json:"auth_mode"` // "none" | "basic" | "keycloak"
	BasicUser string `json:"basic_user"`
	BasicPass string `json:"basic_pass"`
}

// Settings is the full persisted state. GitHubPAT and MCP.BasicPass are
// secrets: stored on disk but never serialised back to the client (handlers
// expose only whether they are set).
type Settings struct {
	Identity  Identity `json:"identity"`
	GitHubPAT string   `json:"github_pat"`
	MCP       MCP      `json:"mcp"`
}

// Store is a JSON-file-backed, mutex-guarded settings store. The file is
// written 0600 since it may hold a token — parity with git's own
// `credential.helper store`.
type Store struct {
	path string
	mu   sync.RWMutex
	data Settings
}

// NewStore loads settings from <stateDir>/settings.json, starting empty when
// the file does not exist yet.
func NewStore(stateDir string) (*Store, error) {
	s := &Store{path: filepath.Join(stateDir, "settings.json")}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read settings: %w", err)
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return fmt.Errorf("parse settings: %w", err)
	}
	return nil
}

// Get returns a copy of the current settings.
func (s *Store) Get() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data
}

// Update replaces the identity and, when pat is non-nil, the GitHub token (an
// empty string clears it). A nil pat leaves the stored token untouched.
func (s *Store) Update(id Identity, pat *string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Identity = id
	if pat != nil {
		s.data.GitHubPAT = *pat
	}
	return s.persist()
}

// UpdateMCP replaces the MCP config. basicPass follows the same *string
// convention as Update's pat: nil leaves the stored value untouched, "" clears
// it, a non-empty value sets it.
func (s *Store) UpdateMCP(m MCP, basicPass *string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.MCP.Enabled = m.Enabled
	s.data.MCP.AuthMode = m.AuthMode
	s.data.MCP.BasicUser = m.BasicUser
	if basicPass != nil {
		s.data.MCP.BasicPass = *basicPass
	}
	return s.persist()
}

func (s *Store) persist() error {
	if dir := filepath.Dir(s.path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir state: %w", err)
		}
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	if err := os.WriteFile(s.path, b, 0o600); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	return nil
}
