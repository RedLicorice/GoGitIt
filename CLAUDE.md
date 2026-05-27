# GoGitIt — Project Context for Claude Code

A headless Git client with a GitHub Desktop–inspired UI. Runs locally as a
single-user binary, or on a server with Keycloak OIDC authentication.

> Think `ungit`, but with a familiar GitHub aesthetic and a Go backend.

---

## Stack

- **Backend**: Go 1.23, [chi](https://github.com/go-chi/chi) router,
  [go-git/v5](https://github.com/go-git/go-git) for git operations,
  [viper](https://github.com/spf13/viper) for config,
  [go-oidc](https://github.com/coreos/go-oidc) + `golang.org/x/oauth2` for Keycloak,
  [coder/websocket](https://github.com/coder/websocket) + `fsnotify` for live updates.
- **Frontend**: Svelte 4 + Vite 5 + Tailwind 3. Plain JS (no TypeScript yet).
- **Auth**: pluggable provider — passthrough (local user) or OIDC (Keycloak).
  Toggled via `auth.enabled` config flag, decided at startup.
- **Deployment**: Multi-stage Dockerfile → Alpine image (carries `git`, which
  `internal/gitext` shells out to). Reverse proxy (Traefik / Caddy / nginx) is
  left to the operator.

## Project layout

```
.
├── cmd/gogitit/main.go           # Entry: load config, init deps, start HTTP
├── internal/
│   ├── api/router.go             # chi router, handlers, CORS
│   ├── api/live.go               # fsnotify watch + WebSocket status push
│   ├── auth/auth.go              # OIDC + passthrough Provider interface
│   ├── config/config.go          # viper loader, env override (operator-set)
│   ├── gh/gh.go                  # GitHub CLI wrapper: `gh auth token`
│   ├── git/git.go                # go-git wrapper: Status, Log, Branches
│   ├── git/diff.go               # structured diffs: CommitDiff, WorktreeDiff
│   ├── git/ops.go                # index ops: Stage, Unstage, Discard, Commit
│   ├── git/remotes.go            # remote CRUD: List, Add, SetURL, Remove
│   ├── git/transport.go          # Fetch/Pull/Push + auth resolution
│   ├── git/branches.go           # branch ops: Create, Switch, Delete, Merge
│   ├── git/graph.go              # whole-repo commit graph for the Tree view
│   ├── gitext/gitext.go          # system-git shell-out: stash, submodule add
│   ├── repo/registry.go          # JSON-persisted repo registry, ParentOf
│   └── settings/settings.go      # JSON store (0600): identity + GitHub PAT
├── web/                          # Svelte SPA
│   ├── src/
│   │   ├── App.svelte            # Layout: TopBar + Sidebar + main
│   │   ├── components/
│   │   │   ├── TopBar.svelte
│   │   │   ├── Sidebar.svelte    # Repo list + add form
│   │   │   ├── RepoView.svelte   # Tabs: Changes | History | Settings
│   │   │   ├── Changes.svelte    # Staged / unstaged / untracked + diff pane
│   │   │   ├── History.svelte    # History|Tree sub-tabs; 3-column history
│   │   │   ├── TreeView.svelte   # ungit-style commit graph (lanes + SVG rail)
│   │   │   ├── RepoSettings.svelte    # Per-repo tab: remotes management
│   │   │   ├── BranchMenu.svelte # Branch chip popover: switch/create/merge/delete
│   │   │   ├── ShaPill.svelte    # Click-to-copy commit SHA pill
│   │   │   ├── DiffViewer.svelte # Renders a FileDiff inline or side-by-side
│   │   │   ├── DiffModeToggle.svelte # Unified / Split switch (diffMode store)
│   │   │   ├── ConfirmDialog.svelte   # Reusable in-app confirmation modal
│   │   │   ├── SettingsModal.svelte   # Identity + GitHub auth + About
│   │   │   ├── ToastContainer.svelte  # App-wide toast notifications
│   │   │   ├── RepoStatusIcon.svelte  # Per-repo sidebar status glyph
│   │   │   └── EmptyState.svelte
│   │   ├── lib/api.js            # fetch wrapper for /api/v1
│   │   ├── lib/stores.js         # writable/derived stores
│   │   ├── lib/sync.js           # app-level fetch/pull/push + toasts
│   │   ├── lib/toasts.js         # toast notification store
│   │   ├── lib/repostatus.js     # per-repo status summaries store
│   │   └── lib/live.js           # WebSocket client for live status updates
│   ├── index.html
│   ├── embed.go                  # //go:embed of dist/ — bundles SPA in binary
│   ├── vite.config.js            # Proxies /api and /auth to :8080
│   └── tailwind.config.js        # GitHub Primer-inspired dark palette
├── Dockerfile                    # 3-stage: web → go → alpine (+git)
├── Makefile
└── config.example.yaml
```

## Architectural conventions

- **Business logic stays in `internal/`** — handlers in `api/` are thin
  wrappers around `git/` and `repo/`. Keep `api/` free of git logic.
- **`auth.Provider` is an interface** with two implementations (passthrough,
  oidcProvider). Never branch on `cfg.Auth.Enabled` outside `auth.New()` —
  the rest of the app sees only the interface.
- **Errors bubble up**. Handlers translate to HTTP status codes; the
  `internal/` packages never write to `http.ResponseWriter`.
- **Repo paths are validated** on add (`gitops.Open(path)` must succeed).
  Never trust a path from the client without opening it as a repo first.
- **The registry is JSON on disk**, single-process, mutex-guarded. If we
  outgrow that, swap `internal/repo/registry.go` for a sqlite-backed impl
  behind the same exported API — don't leak storage details elsewhere.
- **Frontend talks only to `/api/v1`**. The `/auth/*` endpoints are
  redirect-based and reached via `window.location`, not `fetch`.

## Coding style

### Go
- Standard `gofmt` + `go vet`. Run `go mod tidy` after dependency changes.
- Use `log/slog` (already wired in `main.go`), not `log` or `fmt.Println`.
- Errors: wrap with `fmt.Errorf("context: %w", err)`. No sentinel-only returns.
- Handlers: receive deps via closure factories (`func handler(reg *repo.Registry) http.HandlerFunc`), not globals.
- JSON tags are lowercase with underscores: `json:"short_hash"`, matching
  what the frontend expects.

### Svelte / JS
- Components are PascalCase, one per file in `web/src/components/`.
- State lives in `web/src/lib/stores.js`. Components import stores and call
  helper functions there — no direct `fetch` in components.
- Tailwind: use the custom utility classes from `app.css` (`.btn`, `.input`)
  before reaching for raw utilities, so styling stays consistent.
- Use the existing semantic color tokens (`canvas`, `border`, `fg`, `accent`,
  `success`, `danger`, `attention`) — don't add hex literals in components.
- Responsive: below `md` (768px) the UI is a single-column drill stack —
  full-width sidebar drawer, master-detail panes shown one at a time with a
  header back control. Multi-pane flex children need `min-w-0` so wide content
  (diffs) scrolls inside instead of blowing out the layout. The `isMobile`
  store gates the drill-navigation state; `md:` classes do the rest.

## Common tasks

### Run dev environment
```bash
make install-frontend   # once
make tidy               # once, after touching go.mod
make dev-backend        # terminal 1 — Go on :8080, auth disabled
make dev-frontend       # terminal 2 — Vite on :5173, HMR
```
Open <http://localhost:5173>.

### Add an API endpoint
1. Add a method to the relevant `internal/` package (e.g. `git.Stage(repo, paths)`).
2. Write a handler in `internal/api/router.go` that calls it and serializes JSON.
3. Wire the route inside the `api.Route("/api/v1", ...)` block.
4. Expose it on the frontend via `web/src/lib/api.js`.
5. Use it from a component via the store layer.

### Add a Go dependency
```bash
go get github.com/foo/bar
go mod tidy
```

### Build a release artifact
```bash
make build          # produces a self-contained ./bin/gogitit (SPA embedded)
./bin/gogitit
```

## Roadmap

Implemented in the initial scaffold:
- [x] Repo registry (add / list / remove) with JSON persistence
- [x] Working-tree status (staged / unstaged / untracked)
- [x] Commit log (HEAD, up to 100)
- [x] Branch listing (local + remote)
- [x] OIDC auth with Keycloak (togglable)
- [x] Dockerfile (multi-stage; Alpine final image — ships `git`)
- [x] Diff viewer — `internal/git/diff.go` produces structured `FileDiff`
      (hunks + numbered lines, 3-line context cropping). Endpoints:
      `GET /repos/{id}/diff?path=...&staged=0|1` (working-tree, one file) and
      `GET /repos/{id}/commit/{hash}/diff` (every file in a commit). Frontend
      `DiffViewer.svelte` renders inline or side-by-side; mode lives in the
      `diffMode` store. Changes pane shows the diff of a clicked file;
      History is a 3-column layout (commits | Commit Files | diff panel) —
      picking files in the Commit Files column filters the diff panel, which
      otherwise shows the commit message plus every file's diff.
- [x] Stage / unstage / discard — `internal/git/ops.go`: `Stage` (go-git
      `Add`), `Unstage` (direct index edit — go-git has no path-limited reset),
      `Discard` (revert to HEAD; delete files absent from HEAD). Endpoints
      `POST /repos/{id}/{stage,unstage,discard}` take `{"paths":[...]}` and
      return the refreshed status. Changes pane has per-file (hover) and
      per-section bulk action buttons; discard is confirmed via dialog.
- [x] Settings — cog in TopBar opens `SettingsModal`. Git identity
      (`user.name`/`user.email`), GitHub auth (gh CLI token detection + manual
      HTTPS PAT), and an About section. Backend: `internal/settings` (JSON
      store, 0600 — holds the PAT) and `internal/gh` (`gh auth token`).
      `GET`/`PUT /api/v1/settings`; the PAT is never returned, only `pat_set`.
      `gh` works in both local and server mode; toggle via `gh.enabled` config
      (env `GOGITIT_GH_ENABLED`).
- [x] Commit — `git.CreateCommit` (go-git `wt.Commit`); author from the
      settings identity, falling back to the repo/global git config when it is
      unset. `POST /repos/{id}/commit` ({summary, description}) → new commit
      hash + refreshed status. Commit form in `Changes.svelte` is wired
      (enabled only with a summary and staged files). `Status.Detached` flags a
      detached HEAD; committing there needs confirmation via `ConfirmDialog`.
- [x] Post-commit UX — a commit that leaves the working tree clean switches
      `Changes` to the History tab. `GetLog` marks each commit `Pushed` vs
      pending-push: a repo with no remote flags nothing; a remote whose
      tracking ref is absent flags every commit; otherwise per-commit
      reachability. The History list shows an "unpushed" badge.
- [x] Per-repo Settings tab — `RepoView` has a third tab (Changes | History |
      Settings). `RepoSettings.svelte` lists, adds, edits, and removes git
      remotes via `internal/git/remotes.go` and `GET`/`POST`/`PUT`/`DELETE
      /repos/{id}/remotes` (mutating calls return the refreshed list).
- [x] Push / pull / fetch — `internal/git/transport.go`: `Fetch`, `Pull`
      (go-git fetch + `gitext.MergeFF` fast-forward — go-git's `Worktree.Pull`
      half-applies on a dirty tree; a non-fast-forward errors until merge
      support lands), `Push` (current branch; advances the local tracking ref).
      `ResolveAuth` picks auth from the remote URL: gh token for github.com
      HTTPS, else the stored PAT (HTTP basic auth); ssh-agent or a default
      `~/.ssh` key for SSH URLs. `POST /repos/{id}/{fetch,pull,push}`;
      Fetch/Pull/Push buttons live in the `RepoView` tab bar. Deferred:
      per-remote credential override UI, passphrase-protected SSH keys (use
      ssh-agent), surfacing upstream state in the Settings tab, OAuth.
- [x] Toasts & background sync — fetch/pull/push run app-level in `lib/sync.js`
      so an op keeps running (and still reports) after the user switches repos.
      Results surface as toasts (`lib/toasts.js` + `ToastContainer`); a fetch
      that finds commits offers a **Pull** action, a pull that leaves commits
      to push offers **Push** — chainable straight from the toast. `GetStatus`
      now populates `Status.Ahead`/`Behind` (`aheadBehind`).
- [x] Repo-menu status indicators — each sidebar row shows one indicator via a
      priority cascade (`RepoStatusIcon`): running op (spinner + action glyph)
      › last-op error › diverged/merge › uncommitted changes (●N) › unpushed
      (↑N) › behind (↓N) › clean (✓). Backend `GET /repos/statuses` returns a
      per-repo summary; `lib/repostatus.js` holds the `repoStatuses` store,
      refreshed on app load, after sync ops, and after working-tree changes.

- [x] Diverged-HEAD warning — the `RepoView` branch chip shows ahead/behind
      counts (↑N ↓M); when HEAD has diverged from its upstream (both nonzero)
      the chip gets a 4px orange left border plus a tooltip. Driven by the
      `repoStatuses` store.

- [x] Branch create / switch / merge / delete — `BranchMenu.svelte` popover on
      the `RepoView` branch chip (switch, create, per-branch merge /
      delete-with-confirm). Switching with a dirty worktree prompts: bring the
      changes along, or stash them aside (`checkout` takes a `stash` flag).
      Create (at HEAD) and delete are pure ref ops in
      `internal/git/branches.go`. Switch and FF-merge go through
      `internal/gitext` (`git checkout` / `git merge --ff-only`) — go-git's
      worktree checkout/reset prunes untracked files, so the system git binary
      does these. `POST /repos/{id}/{branches,checkout,merge}`,
      `DELETE /repos/{id}/branches/{name}`.
- [x] Commit graph / Tree view — the History view has a History | Tree
      sub-tab. Tree (`TreeView.svelte`) is an ungit-style graph:
      `internal/git/graph.go` walks every commit object plus all refs
      (`GET /repos/{id}/graph`), topologically sorted with reachability flags.
      The frontend assigns lanes and draws an SVG rail; each node is a
      box-label (summary, SHA, author, date) coloured for HEAD, abandoned
      (unreachable) and unpushed commits, with branch / remote / tag chips. The
      object scan is capped at 2000 commits. An abandoned commit carries an
      **Attach branch** action — `POST /repos/{id}/branches` with a `hash` →
      `git.CreateBranchAt` creates a branch there without moving HEAD, rescuing
      dangling work into the normal switch / merge / push flows.
- [x] Live updates — `internal/api/live.go`: an fsnotify watch pushes status
      summaries over a WebSocket (`/api/v1/ws`, `coder/websocket`), debounced
      400ms. It watches each repo's working tree **and** its git directory
      (`.git` + the `refs` subtree; `objects`/`logs` skipped) so commits,
      branch switches and fetches — from GoGitIt, the CLI, or another client —
      all propagate live. Frontend `lib/live.js` connects with auto-reconnect
      and updates the `repoStatuses` store, so sidebar indicators refresh live;
      `Changes` soft-reloads its file list.
- [x] Embedded frontend — `web/embed.go` compiles the built SPA into the binary
      (`//go:embed all:dist`); `internal/api` serves it at `/` with an
      index.html fallback. The binary is self-contained — no `web/dist`
      directory ships alongside. `web/dist/.gitkeep` keeps the embed directory
      present on a fresh checkout, before the frontend is built.
- [x] Stash & detached-HEAD recovery — `internal/gitext` shells to system git
      for operations go-git lacks. Stash: `POST /repos/{id}/stash` and
      `…/stash/pop` (untracked included); Stash / Pop buttons in the Changes
      commit area. Detached-HEAD recovery: Changes offers an inline "create a
      branch" to move the work onto a branch.
- [x] Submodule awareness — `Registry.ParentOf` detects a registered repo
      that is a submodule of another (`.git`-file + path prefix). `/repos`
      carries `parent_id`/`parent_name`; the Sidebar repo row shows a small
      fork-icon + parent name. Pushing a submodule offers a toast action
      "Update parent reference & push" → `POST /repos/{id}/parent-update`
      stages the gitlink in the parent, commits, and pushes it.
- [x] Submodule management — `gitext.ListSubmodules` (parses
      `git submodule status`) and `gitext.SubmoduleUpdate`
      (`--init --recursive [--remote]`). `GET /repos/{id}/submodules` returns
      each entry with state + `registered_id`/`registered_name` for any that
      are already added to GoGitIt; `POST .../submodules/update` runs the
      update. `SubmoduleMenu.svelte` is a popover on the `RepoView` tab bar
      (shown only when the repo has submodules) — per-entry **Open** (if
      registered) or **Add to GoGitIt** (otherwise), and the bulk update
      commands. Submodule (gitlink) diffs are rendered specially by
      `DiffViewer`: synthetic "Subproject commit OLD→NEW" hunk, SHA pills,
      capped log of commits the gitlink moved through, and a one-click
      **Commit & push reference** button → `POST .../submodule-commit-push`
      stages, commits and pushes the gitlink change in the parent.

- [x] Git LFS — per-repo settings in the Repo Settings tab: toggle
      auto-tracking + threshold (default 100 MB). State stored as
      `gogitit.lfs.*` in the repo's local git config. `git.Stage` is
      LFS-aware: when auto-tracking is on, files over the threshold are
      `git lfs track`'d (and `.gitattributes` staged) before `git add`, so
      they land as pointers rather than blobs. Endpoints `GET`/`PUT
      /repos/{id}/lfs` return + accept `{available, hooks_installed, enabled,
      threshold_bytes}`; `hooks_installed` reads the effective `filter.lfs.clean`
      (so a global `git lfs install` is recognised, not just local).

- [x] Built-in MCP server — `internal/api/mcp.go` registers GoGitIt's git
      ops as MCP tools (list_repos, repo_status, repo_log, repo_branches,
      repo_diff, repo_stage, repo_unstage, repo_commit, repo_push, repo_fetch)
      and serves them over HTTP + SSE via `mark3labs/mcp-go` at `/mcp/sse` +
      `/mcp/message` (mounted with `WithStaticBasePath("/mcp")`). Per-app
      config in `settings.MCP`: toggle + auth mode (`none` | `basic` |
      `keycloak`, the last gated on `cfg.Auth.Enabled`). Per-request middleware
      reads settings live so toggling takes effect immediately. Settings UI
      in `SettingsModal` includes a copyable `claude mcp add` snippet that
      bakes in the basic-auth header. TopBar shows an MCP status dot (green
      when enabled). Logout is now an icon button.

Roadmap complete — every planned item is implemented.

Possible future work:
- OAuth (device / web flow) for hosted-git auth.
- True 3-way merge (`internal/git` is fast-forward only; could extend
  `internal/gitext` to shell `git merge`).
- Virtualized Tree view for very large repos.

## Gotchas

- **The git split — `gitext` mutates, go-git reads.** go-git's worktree/index
  code is unreliable: it ignores `core.fileMode`, mishandles `.gitignore`,
  prunes untracked files on checkout/reset, and is not atomic (`Worktree.Pull`
  half-applies on a dirty tree). So **every operation that touches the working
  tree or index goes through `internal/gitext`** (system `git`): status, stage,
  unstage, discard, commit, stash, branch switch, FF-merge, pull's
  fast-forward, submodule gitlink staging. `internal/gitext` is the *only*
  place that execs `git`, serialized per repo (`.git/index.lock` races).
  - **go-git is kept for object-database reads** — log, graph, commit/tree/blob
    diffs, ref listing — and for network transport (fetch, push). Those are
    reliable and pure-Go. Don't migrate them: parsing `git log`/`git diff`
    output back into the structured types is bug-prone for zero gain.
  - The `git` binary is therefore **required** (the Docker image ships it).
    `GetStatus` falls back to go-git's Status only if git is entirely absent;
    the mutating ops do not fall back.
- **Repo paths**: in local mode users add absolute filesystem paths. In server
  mode, paths should be under `storage.repos_dir`. Don't conflate the two.
- **CORS in dev**: the Vite proxy hides cross-origin issues. When testing the
  built binary directly (no proxy), the SPA must be served from the same origin
  as the API or `cors.allowed_origins` must include the SPA origin.
- **OIDC cookie**: the session cookie stores the raw `id_token`. Lifetime is
  capped at 12h regardless of token `exp`. When implementing refresh, store
  the refresh token server-side, not in the cookie.
- **`gh` CLI uses the host login.** `internal/gh` shells to the host's `gh`,
  whose login is the *machine's* identity — in server mode that is shared by
  all users. Allowed in both local and server mode; operators disable it
  globally with `gh.enabled` / `GOGITIT_GH_ENABLED`. Always gate `gh` calls on
  `cfg.GH.Enabled` (pass `gh.Probe(cfg.GH.Enabled)`).

## How to ask Claude Code for help here

- For backend changes, mention the affected `internal/` package by name.
- For frontend changes, name the component file.
- Prefer "implement X end-to-end" requests (registry → handler → API client →
  component) over piecemeal ones — the layers are thin and consistent.
- After non-trivial changes, ask Claude Code to run `go build ./...` and
  `cd web && npm run build` to verify both sides compile.
