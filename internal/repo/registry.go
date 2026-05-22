package repo

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Entry is a registered repository known to the app. Path may be absolute
// (when used locally) or relative to ReposDir (when on the server).
type Entry struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	AddedAt   time.Time `json:"added_at"`
	LastUsed  time.Time `json:"last_used,omitempty"`
}

// Registry persists the list of repos managed by the app to a JSON file.
// Concurrent access is serialized with a mutex; good enough for a
// single-process app on a small registry.
type Registry struct {
	mu       sync.RWMutex
	path     string
	reposDir string
	entries  map[string]*Entry
}

func NewRegistry(stateDir, reposDir string) (*Registry, error) {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir state: %w", err)
	}
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir repos: %w", err)
	}

	r := &Registry{
		path:     filepath.Join(stateDir, "repos.json"),
		reposDir: reposDir,
		entries:  map[string]*Entry{},
	}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Registry) load() error {
	f, err := os.Open(r.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	var list []*Entry
	if err := json.NewDecoder(f).Decode(&list); err != nil {
		return fmt.Errorf("decode registry: %w", err)
	}
	for _, e := range list {
		r.entries[e.ID] = e
	}
	return nil
}

func (r *Registry) save() error {
	list := make([]*Entry, 0, len(r.entries))
	for _, e := range r.entries {
		list = append(list, e)
	}

	tmp := r.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(list); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}

func (r *Registry) List() []*Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Entry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, e)
	}
	return out
}

func (r *Registry) Get(id string) (*Entry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.entries[id]
	if !ok {
		return nil, errors.New("repo not found")
	}
	return e, nil
}

func (r *Registry) Add(name, path string) (*Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if name == "" {
		name = filepath.Base(path)
	}

	e := &Entry{
		ID:      uuid.NewString(),
		Name:    name,
		Path:    path,
		AddedAt: time.Now().UTC(),
	}
	r.entries[e.ID] = e
	if err := r.save(); err != nil {
		delete(r.entries, e.ID)
		return nil, err
	}
	return e, nil
}

func (r *Registry) Remove(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.entries[id]; !ok {
		return errors.New("repo not found")
	}
	delete(r.entries, id)
	return r.save()
}

func (r *Registry) ReposDir() string { return r.reposDir }

// ParentOf returns the registered repo that contains the given repo as a
// submodule (or linked worktree). A submodule has a .git *file* (a gitdir
// pointer) rather than a directory; the parent is the registered repo whose
// path is the longest ancestor of this one.
func (r *Registry) ParentOf(id string) (*Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	child, ok := r.entries[id]
	if !ok {
		return nil, false
	}
	if fi, err := os.Stat(filepath.Join(child.Path, ".git")); err != nil || fi.IsDir() {
		return nil, false // not a submodule / linked worktree
	}
	childPath := filepath.Clean(child.Path) + string(filepath.Separator)
	var best *Entry
	for _, e := range r.entries {
		if e.ID == id {
			continue
		}
		base := filepath.Clean(e.Path) + string(filepath.Separator)
		if strings.HasPrefix(childPath, base) {
			if best == nil || len(e.Path) > len(best.Path) {
				best = e
			}
		}
	}
	return best, best != nil
}
