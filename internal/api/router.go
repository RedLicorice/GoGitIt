package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"

	"github.com/RedLicorice/GoGitIt/internal/auth"
	"github.com/RedLicorice/GoGitIt/internal/config"
	gitops "github.com/RedLicorice/GoGitIt/internal/git"
	"github.com/RedLicorice/GoGitIt/internal/gh"
	"github.com/RedLicorice/GoGitIt/internal/gitext"
	"github.com/RedLicorice/GoGitIt/internal/repo"
	"github.com/RedLicorice/GoGitIt/internal/settings"
	"github.com/RedLicorice/GoGitIt/web"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// NewRouter wires public routes (login/callback/health) and protected
// API routes (everything under /api/v1). The static frontend is served
// from /; in dev mode the Svelte dev server runs separately on :5173
// and talks to this server via the /api proxy.
func NewRouter(cfg *config.Config, authP auth.Provider, registry *repo.Registry, set *settings.Store) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORS.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Public
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Get("/auth/login", authP.LoginHandler())
	r.Get("/auth/callback", authP.CallbackHandler())
	r.Get("/auth/logout", authP.LogoutHandler())

	// Live updates: watches repo working trees, pushes status over WebSocket.
	hub := newLiveHub(registry, cfg)

	// Built-in MCP server (HTTP+SSE). Auth + enable are driven by app settings
	// and checked per-request, so toggling in the UI takes effect immediately.
	mcpH := newMCPHub(registry, set, cfg, authP)
	r.Group(func(g chi.Router) {
		g.Use(mcpH.authMiddleware)
		g.Mount("/mcp", mcpH.sse)
	})

	// Protected API
	r.Route("/api/v1", func(api chi.Router) {
		api.Use(authP.Middleware)

		api.Get("/ws", hub.handler())

		api.Get("/me", authP.MeHandler())

		// Settings
		api.Get("/settings", settingsGet(set, cfg.GH.Enabled))
		api.Put("/settings", settingsPut(set, cfg.GH.Enabled))
		api.Get("/settings/mcp", mcpSettingsGet(set, cfg))
		api.Put("/settings/mcp", mcpSettingsPut(set, cfg))

		// Repos
		api.Get("/repos", listRepos(registry))
		api.Post("/repos", addRepo(registry, hub))
		api.Get("/repos/statuses", reposStatuses(registry))
		api.Delete("/repos/{id}", removeRepo(registry, hub))

		// Repo-scoped
		api.Get("/repos/{id}/status", repoStatus(registry))
		api.Get("/repos/{id}/log", repoLog(registry))
		api.Get("/repos/{id}/graph", repoGraph(registry))
		api.Get("/repos/{id}/branches", repoBranches(registry))
		api.Post("/repos/{id}/branches", repoBranchOp(registry, "create"))
		api.Delete("/repos/{id}/branches/{name}", repoBranchOp(registry, "delete"))
		api.Post("/repos/{id}/checkout", repoBranchOp(registry, "switch"))
		api.Post("/repos/{id}/merge", repoBranchOp(registry, "merge"))
		api.Get("/repos/{id}/diff", repoDiff(registry))
		api.Get("/repos/{id}/commit/{hash}/diff", repoCommitDiff(registry))

		// Index manipulation
		api.Post("/repos/{id}/stage", repoIndexOp(registry, "stage"))
		api.Post("/repos/{id}/unstage", repoIndexOp(registry, "unstage"))
		api.Post("/repos/{id}/discard", repoIndexOp(registry, "discard"))
		api.Post("/repos/{id}/commit", repoCommit(registry, set))

		// Remotes
		api.Get("/repos/{id}/remotes", repoRemotes(registry))
		api.Post("/repos/{id}/remotes", repoAddRemote(registry))
		api.Put("/repos/{id}/remotes/{name}", repoSetRemote(registry))
		api.Delete("/repos/{id}/remotes/{name}", repoRemoveRemote(registry))

		// Transport — fetch / pull / push
		api.Post("/repos/{id}/fetch", repoTransport(registry, set, cfg.GH.Enabled, "fetch"))
		api.Post("/repos/{id}/pull", repoTransport(registry, set, cfg.GH.Enabled, "pull"))
		api.Post("/repos/{id}/push", repoTransport(registry, set, cfg.GH.Enabled, "push"))

		// Stash (system git) + submodule parent update
		api.Get("/repos/{id}/stash", repoStashList(registry))
		api.Post("/repos/{id}/stash", repoStashOp(registry, "stash"))
		api.Post("/repos/{id}/stash/pop", repoStashOp(registry, "pop"))
		api.Post("/repos/{id}/parent-update", repoParentUpdate(registry, set, cfg.GH.Enabled))
		api.Post("/repos/{id}/submodule-commit-push", repoSubmoduleCommitPush(registry, set, cfg.GH.Enabled))
		api.Get("/repos/{id}/submodules", repoSubmodules(registry))
		api.Post("/repos/{id}/submodules/update", repoSubmodulesUpdate(registry))
		api.Get("/repos/{id}/lfs", repoLfsGet(registry))
		api.Put("/repos/{id}/lfs", repoLfsPut(registry))
	})

	// Embedded SPA — serves everything not matched by the routes above.
	if dist, err := web.Dist(); err == nil {
		r.Handle("/*", spaHandler(dist))
	}

	return r
}

// spaHandler serves the embedded single-page app, falling back to index.html
// for any path without a matching file so a deep-link reload still loads.
func spaHandler(dist fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if f, err := dist.Open(name); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		r.URL.Path = "/" // SPA fallback
		fileServer.ServeHTTP(w, r)
	})
}

// ---------- handlers ----------

func listRepos(reg *repo.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		entries := reg.List()
		// Augment each entry with its parent repo, if it is a submodule.
		type repoEntry struct {
			*repo.Entry
			ParentID   string `json:"parent_id,omitempty"`
			ParentName string `json:"parent_name,omitempty"`
		}
		out := make([]repoEntry, 0, len(entries))
		for _, e := range entries {
			re := repoEntry{Entry: e}
			if p, ok := reg.ParentOf(e.ID); ok {
				re.ParentID, re.ParentName = p.ID, p.Name
			}
			out = append(out, re)
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func addRepo(reg *repo.Registry, hub *liveHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if body.Path == "" {
			http.Error(w, "path required", http.StatusBadRequest)
			return
		}
		// Validate it's a real git repo.
		if _, err := gitops.Open(body.Path); err != nil {
			http.Error(w, "not a git repository: "+err.Error(), http.StatusBadRequest)
			return
		}
		e, err := reg.Add(body.Name, body.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		hub.watchRepo(e.ID, e.Path)
		writeJSON(w, http.StatusCreated, e)
	}
}

func removeRepo(reg *repo.Registry, hub *liveHub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := reg.Remove(id); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		hub.unwatchRepo(id)
		w.WriteHeader(http.StatusNoContent)
	}
}

// repoStatusSummary is the compact per-repo status the sidebar needs.
type repoStatusSummary struct {
	ID           string `json:"id"`
	Branch       string `json:"branch,omitempty"`
	Detached     bool   `json:"detached"`
	LocalBranch  bool   `json:"local_branch"`
	Ahead        int    `json:"ahead"`
	Behind       int    `json:"behind"`
	ChangedFiles int    `json:"changed_files"`
	Error        string `json:"error,omitempty"`
}

// reposStatuses returns a status summary for every registered repo, so the
// sidebar can show per-repo indicators without a request per row.
func reposStatuses(reg *repo.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		entries := reg.List()
		out := make([]repoStatusSummary, len(entries))
		var wg sync.WaitGroup
		for i, e := range entries {
			wg.Add(1)
			go func(i int, e *repo.Entry) {
				defer wg.Done()
				s := repoStatusSummary{ID: e.ID}
				repo, err := gitops.Open(e.Path)
				if err != nil {
					s.Error = err.Error()
					out[i] = s
					return
				}
				st, err := gitops.GetStatus(repo)
				if err != nil {
					s.Error = err.Error()
					out[i] = s
					return
				}
				s.Branch = st.Branch
				s.Detached = st.Detached
				s.LocalBranch = st.LocalBranch
				s.Ahead = st.Ahead
				s.Behind = st.Behind
				s.ChangedFiles = len(st.Staged) + len(st.Unstaged) + len(st.Untracked)
				out[i] = s
			}(i, e)
		}
		wg.Wait()
		writeJSON(w, http.StatusOK, out)
	}
}

func repoStatus(reg *repo.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		repo, err := gitops.Open(e.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		st, err := gitops.GetStatus(repo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, st)
	}
}

func repoLog(reg *repo.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		repo, err := gitops.Open(e.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		commits, err := gitops.GetLog(repo, 100)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, commits)
	}
}

// repoGraph serves the whole-repository commit graph for the Tree view.
func repoGraph(reg *repo.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		repo, err := gitops.Open(e.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		graph, err := gitops.GetGraph(repo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, graph)
	}
}

func repoBranches(reg *repo.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		repo, err := gitops.Open(e.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		branches, err := gitops.ListBranches(repo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, branches)
	}
}

// repoDiff serves the working-tree diff of a single file. The `staged` query
// param picks HEAD↔index (staged) vs index↔worktree (unstaged) comparison.
func repoDiff(reg *repo.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, "path required", http.StatusBadRequest)
			return
		}
		staged := r.URL.Query().Get("staged") == "1" || r.URL.Query().Get("staged") == "true"
		repo, err := gitops.Open(e.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fd, err := gitops.WorktreeDiff(repo, path, staged)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, fd)
	}
}

// repoCommitDiff serves the structured diff for every file in a commit.
func repoCommitDiff(reg *repo.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		repo, err := gitops.Open(e.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		diffs, err := gitops.CommitDiff(repo, chi.URLParam(r, "hash"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, diffs)
	}
}

// repoIndexOp handles stage / unstage / discard. Each decodes a {"paths":[...]}
// body, applies the operation, and returns the refreshed working-tree status so
// the frontend needs no follow-up request.
func repoIndexOp(reg *repo.Registry, kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		var body struct {
			Paths []string `json:"paths"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if len(body.Paths) == 0 {
			http.Error(w, "paths required", http.StatusBadRequest)
			return
		}
		repo, err := gitops.Open(e.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		switch kind {
		case "stage":
			err = gitops.Stage(repo, body.Paths)
		case "unstage":
			err = gitops.Unstage(repo, body.Paths)
		case "discard":
			err = gitops.Discard(repo, body.Paths)
		default:
			err = fmt.Errorf("unknown op %q", kind)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		st, err := gitops.GetStatus(repo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, st)
	}
}

// repoCommit records the staged changes as a commit. The author identity comes
// from the settings store, falling back to git config inside gitops.
func repoCommit(reg *repo.Registry, set *settings.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		var body struct {
			Summary     string `json:"summary"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(body.Summary) == "" {
			http.Error(w, "summary required", http.StatusBadRequest)
			return
		}
		repo, err := gitops.Open(e.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		st, err := gitops.GetStatus(repo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(st.Staged) == 0 {
			http.Error(w, "nothing staged to commit", http.StatusBadRequest)
			return
		}

		id := set.Get().Identity
		res, err := gitops.CreateCommit(repo, commitMessage(body.Summary, body.Description), id.Name, id.Email)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fresh, err := gitops.GetStatus(repo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"hash":       res.Hash,
			"short_hash": res.ShortHash,
			"status":     fresh,
		})
	}
}

// commitMessage joins a summary and optional description in the conventional
// git layout: summary line, blank line, body.
func commitMessage(summary, description string) string {
	s := strings.TrimSpace(summary)
	d := strings.TrimSpace(description)
	if d == "" {
		return s
	}
	return s + "\n\n" + d
}

// repoBranchOp handles create / delete / switch / merge. The branch name comes
// from the URL for delete and from the body for the rest. It returns the
// refreshed branch list and working-tree status.
func repoBranchOp(reg *repo.Registry, kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		name := chi.URLParam(r, "name")
		hash := ""     // create: optional commit to start the branch at
		stash := false // switch: stash pending changes before checking out
		if kind != "delete" {
			var body struct {
				Name  string `json:"name"`
				Hash  string `json:"hash"`
				Stash bool   `json:"stash"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "invalid body", http.StatusBadRequest)
				return
			}
			name = body.Name
			hash = body.Hash
			stash = body.Stash
		}
		repo, err := gitops.Open(e.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		switch kind {
		case "create":
			if hash != "" {
				err = gitops.CreateBranchAt(repo, name, hash)
			} else {
				err = gitops.CreateBranch(repo, name)
			}
		case "delete":
			err = gitops.DeleteBranch(repo, name)
		case "switch":
			// System git — go-git's worktree checkout prunes untracked files.
			// Optionally stash pending changes first so the switch lands clean.
			if stash {
				if err = gitext.Stash(e.Path); err != nil {
					break
				}
			}
			err = gitext.Checkout(e.Path, name)
		case "merge":
			err = gitext.MergeFF(e.Path, name)
		default:
			err = fmt.Errorf("unknown branch op %q", kind)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		branches, err := gitops.ListBranches(repo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		st, err := gitops.GetStatus(repo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"branches": branches, "status": st})
	}
}

// repoRemotes lists the repository's configured remotes.
func repoRemotes(reg *repo.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		repo, err := gitops.Open(e.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		list, err := gitops.ListRemotes(repo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// repoAddRemote registers a new remote and returns the refreshed list.
func repoAddRemote(reg *repo.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		var body struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		repo, err := gitops.Open(e.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := gitops.AddRemote(repo, body.Name, body.URL); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		list, err := gitops.ListRemotes(repo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// repoSetRemote updates a remote's URL and returns the refreshed list.
func repoSetRemote(reg *repo.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		var body struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		repo, err := gitops.Open(e.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := gitops.SetRemoteURL(repo, chi.URLParam(r, "name"), body.URL); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		list, err := gitops.ListRemotes(repo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// repoRemoveRemote deletes a remote and returns the refreshed list.
func repoRemoveRemote(reg *repo.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		repo, err := gitops.Open(e.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := gitops.RemoveRemote(repo, chi.URLParam(r, "name")); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		list, err := gitops.ListRemotes(repo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// repoTransport handles fetch / pull / push. It resolves an auth method from
// the current branch's remote URL (gh token, stored PAT, or SSH key) and
// returns the remote name plus the refreshed status.
func repoTransport(reg *repo.Registry, set *settings.Store, ghEnabled bool, kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		repo, err := gitops.Open(e.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		remoteName := gitops.CurrentRemote(repo)
		rm, err := repo.Remote(remoteName)
		if err != nil {
			http.Error(w, fmt.Sprintf("no remote %q configured — add one in the Settings tab", remoteName), http.StatusBadRequest)
			return
		}
		urls := rm.Config().URLs
		if len(urls) == 0 {
			http.Error(w, "remote has no URL", http.StatusBadRequest)
			return
		}

		ghToken := ""
		if ghEnabled {
			ghToken, _ = gh.Token()
		}
		auth, err := gitops.ResolveAuth(urls[0], ghToken, set.Get().GitHubPAT)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		switch kind {
		case "fetch":
			err = gitops.Fetch(repo, remoteName, auth)
		case "pull":
			err = gitops.Pull(repo, remoteName, auth)
		case "push":
			err = gitops.Push(repo, remoteName, auth)
		default:
			err = fmt.Errorf("unknown op %q", kind)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		st, err := gitops.GetStatus(repo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"remote": remoteName, "status": st})
	}
}

// repoStashList returns the number of stash entries.
func repoStashList(reg *repo.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		count, err := gitext.StashCount(e.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"count": count})
	}
}

// repoStashOp stashes all changes or pops the latest stash (system git), then
// returns the refreshed status and stash count.
func repoStashOp(reg *repo.Registry, kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if kind == "stash" {
			err = gitext.Stash(e.Path)
		} else {
			err = gitext.StashPop(e.Path)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		repo, err := gitops.Open(e.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		st, err := gitops.GetStatus(repo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		count, _ := gitext.StashCount(e.Path)
		writeJSON(w, http.StatusOK, map[string]any{"count": count, "status": st})
	}
}

// repoParentUpdate, for a submodule repo, stages its updated commit in the
// parent repo, commits that gitlink bump, and pushes the parent.
func repoParentUpdate(reg *repo.Registry, set *settings.Store, ghEnabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subID := chi.URLParam(r, "id")
		sub, err := reg.Get(subID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		parent, ok := reg.ParentOf(subID)
		if !ok {
			http.Error(w, "this repo is not a submodule of a registered repo", http.StatusBadRequest)
			return
		}
		rel, err := filepath.Rel(parent.Path, sub.Path)
		if err != nil {
			http.Error(w, "cannot resolve submodule path", http.StatusInternalServerError)
			return
		}
		if err := gitext.StageSubmodule(parent.Path, rel); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		prepo, err := gitops.Open(parent.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		st, err := gitops.GetStatus(prepo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(st.Staged) == 0 {
			http.Error(w, "parent reference already matches the submodule", http.StatusBadRequest)
			return
		}
		id := set.Get().Identity
		if _, err := gitops.CreateCommit(prepo, fmt.Sprintf("Update submodule %s", rel), id.Name, id.Email); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		remoteName := gitops.CurrentRemote(prepo)
		rm, err := prepo.Remote(remoteName)
		if err != nil {
			http.Error(w, fmt.Sprintf("parent has no remote %q", remoteName), http.StatusBadRequest)
			return
		}
		urls := rm.Config().URLs
		if len(urls) == 0 {
			http.Error(w, "parent remote has no URL", http.StatusBadRequest)
			return
		}
		ghToken := ""
		if ghEnabled {
			ghToken, _ = gh.Token()
		}
		auth, err := gitops.ResolveAuth(urls[0], ghToken, set.Get().GitHubPAT)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := gitops.Push(prepo, remoteName, auth); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"parent": parent.Name, "remote": remoteName})
	}
}

// repoSubmoduleCommitPush stages a submodule path's updated gitlink in this
// repo, commits, and pushes. Parent-side counterpart to repoParentUpdate —
// invoked from the parent's diff view, takes the submodule path directly so
// the submodule does not need to be registered as a separate repo.
func repoSubmoduleCommitPush(reg *repo.Registry, set *settings.Store, ghEnabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Path == "" {
			http.Error(w, "path required", http.StatusBadRequest)
			return
		}
		if err := gitext.StageSubmodule(e.Path, body.Path); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		repo, err := gitops.Open(e.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		st, err := gitops.GetStatus(repo)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(st.Staged) == 0 {
			http.Error(w, "nothing to commit — gitlink unchanged", http.StatusBadRequest)
			return
		}
		id := set.Get().Identity
		if _, err := gitops.CreateCommit(repo, fmt.Sprintf("Update submodule %s", body.Path), id.Name, id.Email); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		remoteName := gitops.CurrentRemote(repo)
		rm, err := repo.Remote(remoteName)
		if err != nil {
			http.Error(w, fmt.Sprintf("no remote %q", remoteName), http.StatusBadRequest)
			return
		}
		urls := rm.Config().URLs
		if len(urls) == 0 {
			http.Error(w, "remote has no URL", http.StatusBadRequest)
			return
		}
		ghToken := ""
		if ghEnabled {
			ghToken, _ = gh.Token()
		}
		auth, err := gitops.ResolveAuth(urls[0], ghToken, set.Get().GitHubPAT)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := gitops.Push(repo, remoteName, auth); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"path": body.Path, "remote": remoteName})
	}
}

// lfsState is the per-repo LFS state returned by GET/PUT /repos/{id}/lfs.
type lfsState struct {
	Available      bool  `json:"available"`       // git-lfs binary on PATH
	HooksInstalled bool  `json:"hooks_installed"` // filter.lfs.clean wired in this repo
	Enabled        bool  `json:"enabled"`         // gogitit.lfs.enabled (auto-track on stage)
	ThresholdBytes int64 `json:"threshold_bytes"` // gogitit.lfs.threshold
}

func readLfsState(repoPath string) lfsState {
	v, _ := gitext.ConfigGet(repoPath, "gogitit.lfs.enabled")
	threshold := int64(100 * 1024 * 1024)
	if vt, _ := gitext.ConfigGet(repoPath, "gogitit.lfs.threshold"); vt != "" {
		if n, err := strconv.ParseInt(vt, 10, 64); err == nil && n > 0 {
			threshold = n
		}
	}
	clean, _ := gitext.ConfigGet(repoPath, "filter.lfs.clean")
	return lfsState{
		Available:      gitext.LfsAvailable(),
		HooksInstalled: clean != "",
		Enabled:        strings.EqualFold(v, "true"),
		ThresholdBytes: threshold,
	}
}

// repoLfsGet returns the per-repo LFS state — both git's own (hooks installed)
// and GoGitIt's auto-tracking config.
func repoLfsGet(reg *repo.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, readLfsState(e.Path))
	}
}

// repoLfsPut updates the per-repo LFS auto-tracking config. Enabling also runs
// `git lfs install --local` so LFS filters are wired even if they weren't yet.
func repoLfsPut(reg *repo.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		var body struct {
			Enabled        bool  `json:"enabled"`
			ThresholdBytes int64 `json:"threshold_bytes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if body.Enabled && !gitext.LfsAvailable() {
			http.Error(w, "git-lfs is not installed on the host", http.StatusBadRequest)
			return
		}
		if body.ThresholdBytes <= 0 {
			body.ThresholdBytes = 100 * 1024 * 1024
		}
		if err := gitext.ConfigSet(e.Path, "gogitit.lfs.enabled", strconv.FormatBool(body.Enabled)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := gitext.ConfigSet(e.Path, "gogitit.lfs.threshold", strconv.FormatInt(body.ThresholdBytes, 10)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if body.Enabled {
			if err := gitext.LfsInstallLocal(e.Path); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		writeJSON(w, http.StatusOK, readLfsState(e.Path))
	}
}

// repoSubmodules lists a repo's submodules with their state, augmenting each
// entry with the registered repo (if any) that GoGitIt knows for that path.
func repoSubmodules(reg *repo.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		entries, err := gitext.ListSubmodules(e.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Index registered repos by clean path so each submodule can carry its
		// registered_id / registered_name if it has been added to GoGitIt.
		byPath := map[string]*repo.Entry{}
		for _, x := range reg.List() {
			byPath[filepath.Clean(x.Path)] = x
		}
		type row struct {
			gitext.SubmoduleEntry
			AbsPath        string `json:"abs_path"`
			RegisteredID   string `json:"registered_id,omitempty"`
			RegisteredName string `json:"registered_name,omitempty"`
		}
		parent := filepath.Clean(e.Path)
		out := make([]row, 0, len(entries))
		for _, s := range entries {
			abs := filepath.Join(parent, s.Path)
			rr := row{SubmoduleEntry: s, AbsPath: abs}
			if hit, ok := byPath[filepath.Clean(abs)]; ok {
				rr.RegisteredID, rr.RegisteredName = hit.ID, hit.Name
			}
			out = append(out, rr)
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// repoSubmodulesUpdate runs `git submodule update --init --recursive` for the
// repo. Body `{remote: true}` adds --remote (advance each submodule to its
// tracked branch's latest commit).
func repoSubmodulesUpdate(reg *repo.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e, err := reg.Get(chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		var body struct {
			Remote bool `json:"remote"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body) // body optional
		if err := gitext.SubmoduleUpdate(e.Path, body.Remote); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		entries, _ := gitext.ListSubmodules(e.Path)
		writeJSON(w, http.StatusOK, map[string]any{"submodules": entries})
	}
}

// settingsGet returns the user settings, the gh CLI status, and build info.
// The stored GitHub token is never returned — only whether one is set.
func settingsGet(set *settings.Store, ghEnabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		cur := set.Get()
		writeJSON(w, http.StatusOK, map[string]any{
			"identity": cur.Identity,
			"github": map[string]any{
				"pat_set": cur.GitHubPAT != "",
				"gh":      gh.Probe(ghEnabled),
			},
			"about": aboutInfo(),
		})
	}
}

// settingsPut updates the identity and, when github_pat is present in the body,
// the stored token (an empty string clears it; omitting the field keeps it).
func settingsPut(set *settings.Store, ghEnabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Identity  settings.Identity `json:"identity"`
			GitHubPAT *string           `json:"github_pat"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if err := set.Update(body.Identity, body.GitHubPAT); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		settingsGet(set, ghEnabled)(w, r)
	}
}

// aboutInfo reports the app, go-git, and runtime versions via build metadata.
func aboutInfo() map[string]string {
	info := map[string]string{"version": "dev", "go": runtime.Version()}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if bi.Main.Version != "" {
			info["version"] = bi.Main.Version
		}
		for _, d := range bi.Deps {
			if d.Path == "github.com/go-git/go-git/v5" {
				info["go_git"] = d.Version
			}
		}
	}
	return info
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
