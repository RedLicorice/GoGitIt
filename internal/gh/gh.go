// Package gh wraps the GitHub CLI (`gh`) for credential discovery. Shelling out
// to `gh` is the only supported, hack-free way to reuse a developer's existing
// gh login — there is no Go library equivalent. It works in both local and
// server mode; operators can disable it globally via the gh.enabled config.
package gh

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// Status describes whether the gh CLI integration is usable on this host.
type Status struct {
	Enabled       bool `json:"enabled"`       // not disabled by config
	Available     bool `json:"available"`     // gh binary found
	Authenticated bool `json:"authenticated"` // gh has a valid login
}

// Probe reports gh availability. When enabled is false (operator disabled the
// integration via config) it returns immediately without touching the binary.
// It makes no network call — `gh auth token` only reads gh's local config.
func Probe(enabled bool) Status {
	if !enabled {
		return Status{}
	}
	st := Status{Enabled: true}
	if _, err := exec.LookPath("gh"); err != nil {
		return st
	}
	st.Available = true
	if _, err := Token(); err == nil {
		st.Authenticated = true
	}
	return st
}

// Token returns the GitHub token from `gh auth token`. It requires gh to be
// installed and logged in; intended for HTTPS push/pull against GitHub.
func Token() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "gh", "auth", "token").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
