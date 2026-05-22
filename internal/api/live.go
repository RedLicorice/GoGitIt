package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/RedLicorice/GoGitIt/internal/config"
	gitops "github.com/RedLicorice/GoGitIt/internal/git"
	"github.com/RedLicorice/GoGitIt/internal/repo"
	"github.com/coder/websocket"
	"github.com/fsnotify/fsnotify"
)

// debounceDelay coalesces a burst of filesystem events (a save, a checkout)
// into a single status recompute.
const debounceDelay = 400 * time.Millisecond

// liveHub watches registered repos' working trees and pushes status summaries
// to connected WebSocket clients so the UI updates without a manual refresh.
type liveHub struct {
	reg            *repo.Registry
	originPatterns []string

	mu      sync.Mutex
	clients map[chan []byte]struct{}

	watcher *fsnotify.Watcher
	wmu     sync.Mutex
	dirRepo map[string]string // watched directory -> repo id

	dmu      sync.Mutex
	debounce map[string]*time.Timer // repo id -> pending recompute
}

func newLiveHub(reg *repo.Registry, cfg *config.Config) *liveHub {
	h := &liveHub{
		reg:            reg,
		originPatterns: originPatterns(cfg),
		clients:        map[chan []byte]struct{}{},
		dirRepo:        map[string]string{},
		debounce:       map[string]*time.Timer{},
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error("live: cannot create fs watcher", "err", err)
		return h
	}
	h.watcher = w
	for _, e := range reg.List() {
		h.watchRepo(e.ID, e.Path)
	}
	go h.run()
	return h
}

// originPatterns turns the configured CORS origins into host patterns the
// WebSocket handshake will accept (the dev SPA connects through the Vite proxy).
func originPatterns(cfg *config.Config) []string {
	out := []string{}
	for _, o := range cfg.CORS.AllowedOrigins {
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			out = append(out, u.Host)
		}
	}
	return out
}

// watchRepo adds fs watches for a repo: its working tree (so file edits push
// updates) plus the parts of the git directory that change on commit, branch
// switch and fetch — .git itself (index, HEAD, FETCH_HEAD, packed-refs…) and
// the refs subtree. Those touch no worktree file, so without watching them a
// commit on one client would never reach the others.
func (h *liveHub) watchRepo(id, root string) {
	if h.watcher == nil {
		return
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return
	}
	// Working tree, skipping the .git entry (handled separately below).
	_ = filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if d.Name() == ".git" {
			return filepath.SkipDir
		}
		h.addWatch(id, p)
		return nil
	})
	// Git directory — skip the noisy objects/ and logs/ trees.
	gitDir := h.gitDir(abs)
	if gitDir == "" {
		return
	}
	h.addWatch(id, gitDir)
	_ = filepath.WalkDir(filepath.Join(gitDir, "refs"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		h.addWatch(id, p)
		return nil
	})
}

// addWatch registers one directory with the fs watcher.
func (h *liveHub) addWatch(id, dir string) {
	h.wmu.Lock()
	h.dirRepo[dir] = id
	h.wmu.Unlock()
	if err := h.watcher.Add(dir); err != nil {
		slog.Warn("live: watch failed", "dir", dir, "err", err)
	}
}

// gitDir resolves a repo's git directory — normally <root>/.git, but for a
// submodule or linked worktree .git is a file pointing elsewhere.
func (h *liveHub) gitDir(root string) string {
	p := filepath.Join(root, ".git")
	fi, err := os.Stat(p)
	if err != nil {
		return ""
	}
	if fi.IsDir() {
		return p
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(data)), "gitdir:"))
	if target == "" {
		return ""
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	return filepath.Clean(target)
}

func (h *liveHub) unwatchRepo(id string) {
	if h.watcher == nil {
		return
	}
	h.wmu.Lock()
	defer h.wmu.Unlock()
	for dir, rid := range h.dirRepo {
		if rid == id {
			_ = h.watcher.Remove(dir)
			delete(h.dirRepo, dir)
		}
	}
}

func (h *liveHub) run() {
	for {
		select {
		case ev, ok := <-h.watcher.Events:
			if !ok {
				return
			}
			h.handleEvent(ev)
		case err, ok := <-h.watcher.Errors:
			if !ok {
				return
			}
			slog.Warn("live: fs watcher error", "err", err)
		}
	}
}

func (h *liveHub) handleEvent(ev fsnotify.Event) {
	h.wmu.Lock()
	id := h.dirRepo[filepath.Dir(ev.Name)]
	if id == "" {
		id = h.dirRepo[ev.Name]
	}
	h.wmu.Unlock()
	if id == "" {
		return
	}
	// A new directory needs its own watch.
	if ev.Op&fsnotify.Create != 0 {
		if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() && filepath.Base(ev.Name) != ".git" {
			h.watchRepo(id, ev.Name)
		}
	}
	h.scheduleRecompute(id)
}

func (h *liveHub) scheduleRecompute(id string) {
	h.dmu.Lock()
	defer h.dmu.Unlock()
	if t := h.debounce[id]; t != nil {
		t.Stop()
	}
	h.debounce[id] = time.AfterFunc(debounceDelay, func() {
		h.broadcast(mustJSON(map[string]any{
			"type":   "status",
			"status": h.summary(id),
		}))
	})
}

// summary recomputes the compact status for a repo.
func (h *liveHub) summary(id string) repoStatusSummary {
	s := repoStatusSummary{ID: id}
	e, err := h.reg.Get(id)
	if err != nil {
		s.Error = err.Error()
		return s
	}
	repo, err := gitops.Open(e.Path)
	if err != nil {
		s.Error = err.Error()
		return s
	}
	st, err := gitops.GetStatus(repo)
	if err != nil {
		s.Error = err.Error()
		return s
	}
	s.Branch = st.Branch
	s.Detached = st.Detached
	s.LocalBranch = st.LocalBranch
	s.Ahead = st.Ahead
	s.Behind = st.Behind
	s.ChangedFiles = len(st.Staged) + len(st.Unstaged) + len(st.Untracked)
	return s
}

func (h *liveHub) broadcast(msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default: // slow client — drop this update
		}
	}
}

func (h *liveHub) addClient(ch chan []byte) {
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
}

func (h *liveHub) removeClient(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

// handler upgrades a request to a WebSocket and streams status updates. The
// socket is push-only — nothing the client sends is acted on.
func (h *liveHub) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: h.originPatterns,
		})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		ctx := conn.CloseRead(r.Context())

		send := make(chan []byte, 16)
		h.addClient(send)
		defer h.removeClient(send)

		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-send:
				wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
				err := conn.Write(wctx, websocket.MessageText, msg)
				cancel()
				if err != nil {
					return
				}
			}
		}
	}
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
