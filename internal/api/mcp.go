package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/RedLicorice/GoGitIt/internal/auth"
	"github.com/RedLicorice/GoGitIt/internal/config"
	gitops "github.com/RedLicorice/GoGitIt/internal/git"
	"github.com/RedLicorice/GoGitIt/internal/gh"
	"github.com/RedLicorice/GoGitIt/internal/repo"
	"github.com/RedLicorice/GoGitIt/internal/settings"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// mcpHub wires GoGitIt's git operations as MCP tools, served over HTTP+SSE so
// external LLM clients (Claude Code, Claude Desktop, …) can drive the app.
// Auth is decided per-request from settings.MCP.
type mcpHub struct {
	reg   *repo.Registry
	set   *settings.Store
	cfg   *config.Config
	authP auth.Provider
	sse   *mcpserver.SSEServer
}

func newMCPHub(reg *repo.Registry, set *settings.Store, cfg *config.Config, authP auth.Provider) *mcpHub {
	srv := mcpserver.NewMCPServer("gogitit", "0.1.0")
	h := &mcpHub{reg: reg, set: set, cfg: cfg, authP: authP}
	h.registerTools(srv)
	// Static base path so the SSE server emits /mcp/message?sessionId=… as the
	// endpoint URL, matching where chi mounts us.
	h.sse = mcpserver.NewSSEServer(srv, mcpserver.WithStaticBasePath("/mcp"))
	return h
}

// registerTools defines the MCP surface — read tools first, then mutating.
func (h *mcpHub) registerTools(s *mcpserver.MCPServer) {
	s.AddTool(mcp.NewTool("list_repos",
		mcp.WithDescription("List all repositories registered in GoGitIt."),
	), h.toolListRepos)

	s.AddTool(mcp.NewTool("repo_status",
		mcp.WithDescription("Working-tree status of a repo: branch, staged / unstaged / untracked file lists, ahead / behind counts."),
		mcp.WithString("repo_id", mcp.Required(), mcp.Description("Repo ID from list_repos.")),
	), h.toolRepoStatus)

	s.AddTool(mcp.NewTool("repo_log",
		mcp.WithDescription("Commit log of HEAD (up to 100 commits)."),
		mcp.WithString("repo_id", mcp.Required()),
	), h.toolRepoLog)

	s.AddTool(mcp.NewTool("repo_branches",
		mcp.WithDescription("Local and remote-tracking branches."),
		mcp.WithString("repo_id", mcp.Required()),
	), h.toolRepoBranches)

	s.AddTool(mcp.NewTool("repo_diff",
		mcp.WithDescription("Structured working-tree diff of a single file."),
		mcp.WithString("repo_id", mcp.Required()),
		mcp.WithString("path", mcp.Required(), mcp.Description("Path relative to the repo root.")),
		mcp.WithBoolean("staged", mcp.Description("true → HEAD↔index (what a commit would record); false → index↔worktree.")),
	), h.toolRepoDiff)

	s.AddTool(mcp.NewTool("repo_stage",
		mcp.WithDescription("Stage one or more paths."),
		mcp.WithString("repo_id", mcp.Required()),
		mcp.WithArray("paths", mcp.Required(), mcp.Description("Paths to stage, relative to the repo root.")),
	), h.toolRepoStage)

	s.AddTool(mcp.NewTool("repo_unstage",
		mcp.WithDescription("Unstage one or more paths."),
		mcp.WithString("repo_id", mcp.Required()),
		mcp.WithArray("paths", mcp.Required()),
	), h.toolRepoUnstage)

	s.AddTool(mcp.NewTool("repo_commit",
		mcp.WithDescription("Create a commit from the staged index. Author identity comes from app settings or git config."),
		mcp.WithString("repo_id", mcp.Required()),
		mcp.WithString("summary", mcp.Required(), mcp.Description("Commit summary (subject line).")),
		mcp.WithString("description", mcp.Description("Optional commit body.")),
	), h.toolRepoCommit)

	s.AddTool(mcp.NewTool("repo_push",
		mcp.WithDescription("Push the current branch to its tracked remote."),
		mcp.WithString("repo_id", mcp.Required()),
	), h.toolRepoPush)

	s.AddTool(mcp.NewTool("repo_fetch",
		mcp.WithDescription("Fetch updates from the repo's remote."),
		mcp.WithString("repo_id", mcp.Required()),
	), h.toolRepoFetch)
}

// openRepoFromReq resolves a repo_id arg and opens the repo. On error, returns
// a tool error result that the handler can return directly.
func (h *mcpHub) openRepoFromReq(req mcp.CallToolRequest) (*repo.Entry, *gogit.Repository, *mcp.CallToolResult) {
	id, err := req.RequireString("repo_id")
	if err != nil {
		return nil, nil, mcp.NewToolResultErrorFromErr("repo_id", err)
	}
	e, err := h.reg.Get(id)
	if err != nil {
		return nil, nil, mcp.NewToolResultErrorFromErr("repo not found", err)
	}
	r, err := gitops.Open(e.Path)
	if err != nil {
		return nil, nil, mcp.NewToolResultErrorFromErr("open repo", err)
	}
	return e, r, nil
}

// --- tool handlers ---

func (h *mcpHub) toolListRepos(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultJSON(h.reg.List())
}

func (h *mcpHub) toolRepoStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_, r, errRes := h.openRepoFromReq(req)
	if errRes != nil {
		return errRes, nil
	}
	st, err := gitops.GetStatus(r)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("status", err), nil
	}
	return mcp.NewToolResultJSON(st)
}

func (h *mcpHub) toolRepoLog(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_, r, errRes := h.openRepoFromReq(req)
	if errRes != nil {
		return errRes, nil
	}
	commits, err := gitops.GetLog(r, 100)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("log", err), nil
	}
	return mcp.NewToolResultJSON(commits)
}

func (h *mcpHub) toolRepoBranches(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_, r, errRes := h.openRepoFromReq(req)
	if errRes != nil {
		return errRes, nil
	}
	bs, err := gitops.ListBranches(r)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("branches", err), nil
	}
	return mcp.NewToolResultJSON(bs)
}

func (h *mcpHub) toolRepoDiff(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_, r, errRes := h.openRepoFromReq(req)
	if errRes != nil {
		return errRes, nil
	}
	path, err := req.RequireString("path")
	if err != nil {
		return mcp.NewToolResultErrorFromErr("path", err), nil
	}
	staged, _ := req.RequireBool("staged") // optional; false on absence
	fd, err := gitops.WorktreeDiff(r, path, staged)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("diff", err), nil
	}
	return mcp.NewToolResultJSON(fd)
}

func (h *mcpHub) toolRepoStage(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_, r, errRes := h.openRepoFromReq(req)
	if errRes != nil {
		return errRes, nil
	}
	paths, err := req.RequireStringSlice("paths")
	if err != nil {
		return mcp.NewToolResultErrorFromErr("paths", err), nil
	}
	if err := gitops.Stage(r, paths); err != nil {
		return mcp.NewToolResultErrorFromErr("stage", err), nil
	}
	st, _ := gitops.GetStatus(r)
	return mcp.NewToolResultJSON(st)
}

func (h *mcpHub) toolRepoUnstage(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_, r, errRes := h.openRepoFromReq(req)
	if errRes != nil {
		return errRes, nil
	}
	paths, err := req.RequireStringSlice("paths")
	if err != nil {
		return mcp.NewToolResultErrorFromErr("paths", err), nil
	}
	if err := gitops.Unstage(r, paths); err != nil {
		return mcp.NewToolResultErrorFromErr("unstage", err), nil
	}
	st, _ := gitops.GetStatus(r)
	return mcp.NewToolResultJSON(st)
}

func (h *mcpHub) toolRepoCommit(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_, r, errRes := h.openRepoFromReq(req)
	if errRes != nil {
		return errRes, nil
	}
	summary, err := req.RequireString("summary")
	if err != nil {
		return mcp.NewToolResultErrorFromErr("summary", err), nil
	}
	args := req.GetArguments()
	desc, _ := args["description"].(string)
	msg := summary
	if desc != "" {
		msg = summary + "\n\n" + desc
	}
	id := h.set.Get().Identity
	res, err := gitops.CreateCommit(r, msg, id.Name, id.Email)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("commit", err), nil
	}
	return mcp.NewToolResultJSON(res)
}

func (h *mcpHub) toolRepoPush(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_, r, errRes := h.openRepoFromReq(req)
	if errRes != nil {
		return errRes, nil
	}
	auth, err := h.transportAuth(r)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("auth", err), nil
	}
	remoteName := gitops.CurrentRemote(r)
	if err := gitops.Push(r, remoteName, auth); err != nil {
		return mcp.NewToolResultErrorFromErr("push", err), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("pushed to %s", remoteName)), nil
}

func (h *mcpHub) toolRepoFetch(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	_, r, errRes := h.openRepoFromReq(req)
	if errRes != nil {
		return errRes, nil
	}
	auth, err := h.transportAuth(r)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("auth", err), nil
	}
	remoteName := gitops.CurrentRemote(r)
	if err := gitops.Fetch(r, remoteName, auth); err != nil {
		return mcp.NewToolResultErrorFromErr("fetch", err), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("fetched from %s", remoteName)), nil
}

// transportAuth picks transport auth from the current remote URL, mirroring
// what the HTTP API handlers do.
func (h *mcpHub) transportAuth(r *gogit.Repository) (transport.AuthMethod, error) {
	remoteName := gitops.CurrentRemote(r)
	rm, err := r.Remote(remoteName)
	if err != nil {
		return nil, fmt.Errorf("no remote %q: %w", remoteName, err)
	}
	urls := rm.Config().URLs
	if len(urls) == 0 {
		return nil, fmt.Errorf("remote %q has no URL", remoteName)
	}
	ghToken := ""
	if h.cfg.GH.Enabled {
		ghToken, _ = gh.Token()
	}
	return gitops.ResolveAuth(urls[0], ghToken, h.set.Get().GitHubPAT)
}

// mcpStateView is what the settings UI sees — secrets are exposed only as
// "_set" booleans, the same pattern as the GitHub PAT.
type mcpStateView struct {
	Enabled            bool   `json:"enabled"`
	AuthMode           string `json:"auth_mode"`
	BasicUser          string `json:"basic_user"`
	BasicPassSet       bool   `json:"basic_pass_set"`
	KeycloakAvailable  bool   `json:"keycloak_available"`
}

func viewOf(m settings.MCP, cfg *config.Config) mcpStateView {
	mode := m.AuthMode
	if mode == "" {
		mode = "none"
	}
	return mcpStateView{
		Enabled:           m.Enabled,
		AuthMode:          mode,
		BasicUser:         m.BasicUser,
		BasicPassSet:      m.BasicPass != "",
		KeycloakAvailable: cfg.Auth.Enabled,
	}
}

// mcpSettingsGet returns the current MCP settings (without exposing the basic
// password).
func mcpSettingsGet(set *settings.Store, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, viewOf(set.Get().MCP, cfg))
	}
}

// mcpSettingsPut updates the MCP settings. basic_pass follows the *string
// convention: omit to leave untouched, empty to clear, value to set.
func mcpSettingsPut(set *settings.Store, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Enabled   bool    `json:"enabled"`
			AuthMode  string  `json:"auth_mode"`
			BasicUser string  `json:"basic_user"`
			BasicPass *string `json:"basic_pass,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		switch body.AuthMode {
		case "", "none", "basic", "keycloak":
		default:
			http.Error(w, "invalid auth_mode", http.StatusBadRequest)
			return
		}
		if body.AuthMode == "" {
			body.AuthMode = "none"
		}
		if body.AuthMode == "keycloak" && !cfg.Auth.Enabled {
			http.Error(w, "keycloak mode needs the host's OIDC auth to be enabled (auth.enabled in config)", http.StatusBadRequest)
			return
		}
		if body.Enabled && body.AuthMode == "basic" {
			if body.BasicUser == "" || (body.BasicPass != nil && *body.BasicPass == "" && set.Get().MCP.BasicPass == "") {
				http.Error(w, "basic auth needs a username and a password", http.StatusBadRequest)
				return
			}
		}
		err := set.UpdateMCP(settings.MCP{
			Enabled:   body.Enabled,
			AuthMode:  body.AuthMode,
			BasicUser: body.BasicUser,
		}, body.BasicPass)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, viewOf(set.Get().MCP, cfg))
	}
}

// authMiddleware enforces settings.MCP on every request to /mcp/*. It is read
// per-request so toggling auth in the UI takes effect immediately, without
// re-mounting routes.
func (h *mcpHub) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := h.set.Get().MCP
		if !m.Enabled {
			http.Error(w, "MCP disabled", http.StatusServiceUnavailable)
			return
		}
		switch m.AuthMode {
		case "basic":
			u, p, ok := r.BasicAuth()
			if !ok ||
				subtle.ConstantTimeCompare([]byte(u), []byte(m.BasicUser)) != 1 ||
				subtle.ConstantTimeCompare([]byte(p), []byte(m.BasicPass)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="gogitit-mcp"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		case "keycloak":
			// Reuse the OIDC provider's request middleware; requires a session
			// cookie. Browser-bound clients only — for non-browser MCP clients
			// (Claude Code etc.) use "basic" mode.
			h.authP.Middleware(next).ServeHTTP(w, r)
			return
		case "none", "":
			// no auth
		default:
			http.Error(w, "MCP auth mode misconfigured", http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(w, r)
	})
}
