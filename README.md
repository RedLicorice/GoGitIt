# GoGitIt

A headless Git client with a GitHub Desktop–inspired UI. Runs locally as a
single-user binary, or on a server with Keycloak OIDC authentication.

> Like `ungit`, but with a familiar GitHub aesthetic and a Go backend.

## Stack

- **Backend**: Go + chi + go-git + viper + go-oidc
- **Frontend**: Svelte + Vite + Tailwind CSS
- **Auth**: pluggable — none (local) or Keycloak OIDC (server)
- **Deployment**: Docker image, Traefik labels included

## Quick start (local dev)

Requirements: Go 1.22+, Node 20+.

```bash
# 1. Install dependencies
make install-frontend
make tidy

# 2. Run backend (terminal 1) — auth disabled by default
make dev-backend

# 3. Run frontend (terminal 2) — Vite dev server with HMR
make dev-frontend
```

Open <http://localhost:5173>. The Vite dev server proxies `/api/*` and
`/auth/*` to the Go backend on `:8080`.

## Release build

```bash
make build
./bin/gogitit
```

Produces a single binary in `bin/gogitit` with the SPA bundled in `web/dist`
(served by the Go binary in production).

## Server deployment

The included `Dockerfile` builds a distroless image (~25 MB). Run it however
you like — bare `docker run`, your own `docker-compose.yml`, Kubernetes, etc.

For the target host `gogitit.example.com`, configure your reverse proxy to
forward to the container's port 8080 and set these env vars on the container:

```env
GOGITIT_SERVER_ADDR=:8080
GOGITIT_AUTH_ENABLED=true
GOGITIT_AUTH_ISSUER=https://auth.example.com/realms/myrealm
GOGITIT_AUTH_CLIENT_ID=gogitit
GOGITIT_AUTH_CLIENT_SECRET=<from-keycloak>
GOGITIT_AUTH_REDIRECT_URL=https://gogitit.example.com/auth/callback
GOGITIT_AUTH_COOKIE_DOMAIN=gogitit.example.com
GOGITIT_AUTH_COOKIE_SECURE=true
GOGITIT_STORAGE_REPOS_DIR=/data/repos
GOGITIT_STORAGE_STATE_DIR=/data/state
```

Mount a persistent volume at `/data` so repos and the registry survive
container restarts.

Keycloak client requirements:

- Client ID: `gogitit`
- Access type: **confidential** (client authentication ON)
- Standard flow enabled
- Valid redirect URI: `https://gogitit.example.com/auth/callback`
- Web origins: `https://gogitit.example.com`

## Configuration

Settings come from (in order of precedence):

1. Environment variables prefixed `GOGITIT_` (nested keys with `_`)
2. `config.yaml` in CWD or `/etc/gogitit/`
3. Built-in defaults

See `config.example.yaml` for all keys.

## Project structure

```
.
├── cmd/gogitit/          # main entry point
├── internal/
│   ├── api/              # HTTP router + handlers
│   ├── auth/             # OIDC provider (Keycloak) + passthrough
│   ├── config/           # viper config loader
│   ├── git/              # go-git wrapper (status, log, branches)
│   └── repo/             # persistent repo registry (JSON)
├── web/                  # Svelte SPA
│   ├── src/
│   │   ├── components/   # Sidebar, TopBar, RepoView, Changes, History
│   │   ├── lib/          # api client, stores
│   │   ├── App.svelte
│   │   └── main.js
│   ├── package.json
│   ├── vite.config.js
│   └── tailwind.config.js
├── .claude/settings.json # Claude Code permissions for this project
├── CLAUDE.md             # Claude Code project context
├── Dockerfile
├── Makefile
└── config.example.yaml
```

## Working with Claude Code

This repo ships with a `CLAUDE.md` (project context: stack, conventions,
architectural decisions, roadmap) and `.claude/settings.json` (pre-approved
tool permissions for common dev tasks). To continue development:

```bash
claude
```

from the repo root. See [Claude Code docs](https://docs.claude.com/en/docs/claude-code/overview)
for installation.

## Roadmap

This scaffold covers the foundation. Implemented:

- [x] Repo registry (add / list / remove)
- [x] Status (staged / unstaged / untracked)
- [x] Commit log
- [x] Branch listing
- [x] OIDC auth with Keycloak (togglable)
- [x] Traefik-ready Docker deployment

Coming next:

- [ ] Diff viewer (side-by-side)
- [ ] Stage / unstage / commit
- [ ] Push / pull / fetch with credentials
- [ ] Branch create / switch / merge / delete
- [ ] Commit graph visualization
- [ ] WebSocket live updates
- [ ] Embedded frontend (single binary release)

## License

TBD
