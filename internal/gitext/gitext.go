// Package gitext implements git operations by shelling out to the system
// `git` binary. go-git's worktree/index code mishandles core.fileMode,
// .gitignore and untracked files and is not atomic, so every operation that
// touches the working tree or index goes through here; go-git is kept only for
// object-database reads (log, diff, graph). The dependency on an external
// `git` is therefore real — the binary must be on PATH (see Available).
package gitext

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Available reports whether a usable `git` binary is on PATH.
func Available() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// run executes `git -C <repoPath> <args...>` and returns combined output.
func run(repoPath string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	full := append([]string{"-C", repoPath}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	// LC_ALL=C → consistent English error text; GIT_TERMINAL_PROMPT=0 → never
	// block on an interactive credential prompt.
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		// Report the subcommand only — never the full argument list, which for
		// e.g. `git add` with many paths makes the error unreadable.
		return "", fmt.Errorf("git %s: %s", subcommand(args), msg)
	}
	return string(out), nil
}

// subcommand returns the git subcommand from an argument list, skipping leading
// flags (and `-c key=val` config pairs).
func subcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		if !strings.HasPrefix(args[i], "-") {
			return args[i]
		}
		if args[i] == "-c" {
			i++ // also skip the config value
		}
	}
	return "git"
}

// Per-repo serialization: concurrent git processes race on .git/index.lock.
// Mutating operations take the repo's lock; read-only ones (Status…) do not.
var (
	repoLocksMu sync.Mutex
	repoLocks   = map[string]*sync.Mutex{}
)

func repoLock(repoPath string) *sync.Mutex {
	repoLocksMu.Lock()
	defer repoLocksMu.Unlock()
	m := repoLocks[repoPath]
	if m == nil {
		m = &sync.Mutex{}
		repoLocks[repoPath] = m
	}
	return m
}

// runLocked is run() serialized per repo — for index/worktree-mutating commands.
func runLocked(repoPath string, args ...string) (string, error) {
	l := repoLock(repoPath)
	l.Lock()
	defer l.Unlock()
	return run(repoPath, args...)
}

// hasHEAD reports whether the repo has a resolvable HEAD (a born branch).
func hasHEAD(repoPath string) bool {
	_, err := run(repoPath, "rev-parse", "--verify", "-q", "HEAD")
	return err == nil
}

// --- index / worktree mutation ---

// Stage adds paths to the index.
func Stage(repoPath string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	_, err := runLocked(repoPath, append([]string{"add", "--"}, paths...)...)
	return err
}

// Unstage resets paths in the index back to HEAD, keeping worktree changes. On
// an unborn branch there is no HEAD to restore from, so entries are dropped.
func Unstage(repoPath string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	verb := []string{"restore", "--staged", "--"}
	if !hasHEAD(repoPath) {
		verb = []string{"rm", "--cached", "-q", "--"}
	}
	_, err := runLocked(repoPath, append(verb, paths...)...)
	return err
}

// Restore reverts tracked paths to HEAD in both the index and the worktree.
func Restore(repoPath string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	_, err := runLocked(repoPath,
		append([]string{"restore", "--source=HEAD", "--staged", "--worktree", "--"}, paths...)...)
	return err
}

// Clean deletes untracked paths from the worktree.
func Clean(repoPath string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	_, err := runLocked(repoPath, append([]string{"clean", "-fd", "--"}, paths...)...)
	return err
}

// Commit records the staged index as a new commit with the given author
// identity and returns the new commit hash. Hooks run, as on the command line.
func Commit(repoPath, message, name, email string) (string, error) {
	l := repoLock(repoPath)
	l.Lock()
	defer l.Unlock()
	args := []string{}
	if name != "" {
		args = append(args, "-c", "user.name="+name)
	}
	if email != "" {
		args = append(args, "-c", "user.email="+email)
	}
	args = append(args, "commit", "-m", message)
	if _, err := run(repoPath, args...); err != nil {
		return "", err
	}
	out, err := run(repoPath, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// --- stash ---

// Stash saves all changes — including untracked files — to a new stash entry.
func Stash(repoPath string) error {
	out, err := runLocked(repoPath, "stash", "push", "--include-untracked")
	if err != nil {
		return err
	}
	if strings.Contains(out, "No local changes to save") {
		return fmt.Errorf("no changes to stash")
	}
	return nil
}

// StashPop restores the most recent stash entry and drops it.
func StashPop(repoPath string) error {
	_, err := runLocked(repoPath, "stash", "pop")
	return err
}

// StashCount returns the number of stash entries.
func StashCount(repoPath string) (int, error) {
	out, err := run(repoPath, "stash", "list")
	if err != nil {
		return 0, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return 0, nil
	}
	return strings.Count(out, "\n") + 1, nil
}

// --- working-tree status ---

// StatusFile is one changed file from `git status`.
type StatusFile struct {
	Path string
	Code string // M, A, D, R, C, U
}

// StatusResult is the parsed working-tree status.
type StatusResult struct {
	Staged    []StatusFile
	Unstaged  []StatusFile
	Untracked []string
}

// Status returns the working-tree status from `git status --porcelain`. Unlike
// go-git's Worktree.Status it honours core.fileMode, .gitignore, exclude files
// and .gitattributes — i.e. it matches what the user sees on the command line.
//
// --no-optional-locks stops `git status` from writing .git/index (its stat
// cache refresh). Status runs constantly (every /status request, every live
// recompute); without this the index write trips the live hub's .git watch and
// feeds an endless recompute→status→write→recompute loop.
func Status(repoPath string) (*StatusResult, error) {
	out, err := run(repoPath, "--no-optional-locks", "status", "--porcelain", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	res := &StatusResult{}
	tokens := strings.Split(out, "\x00")
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		if len(t) < 4 {
			continue
		}
		x, y, path := t[0], t[1], t[3:]
		// A rename/copy entry is followed by its original path token.
		if x == 'R' || x == 'C' || y == 'R' || y == 'C' {
			i++
		}
		if x == '?' && y == '?' {
			res.Untracked = append(res.Untracked, path)
			continue
		}
		if x != ' ' && x != '?' {
			res.Staged = append(res.Staged, StatusFile{Path: path, Code: string(x)})
		}
		if y != ' ' && y != '?' {
			res.Unstaged = append(res.Unstaged, StatusFile{Path: path, Code: string(y)})
		}
	}
	return res, nil
}

// --- branch / merge ---

// Checkout switches to an existing branch. Unlike go-git's worktree checkout,
// system git preserves untracked files and refuses (rather than discards) when
// the switch would overwrite local work.
func Checkout(repoPath, branch string) error {
	_, err := runLocked(repoPath, "checkout", branch)
	return err
}

// MergeFF fast-forwards the current branch to the named branch. It errors when
// a real (non-fast-forward) merge would be required.
func MergeFF(repoPath, branch string) error {
	_, err := runLocked(repoPath, "merge", "--ff-only", branch)
	return err
}

// SubmoduleEntry describes one submodule's path + state, parsed from
// `git submodule status`.
type SubmoduleEntry struct {
	Path         string `json:"path"`
	Hash         string `json:"hash"`           // recorded commit SHA
	Initialized  bool   `json:"initialized"`    // false when not yet checked out (line starts with '-')
	OutOfSync    bool   `json:"out_of_sync"`    // true when worktree commit ≠ recorded (starts with '+')
	HasConflicts bool   `json:"has_conflicts"`  // merge conflicts in the submodule (starts with 'U')
}

// ListSubmodules returns the repo's submodules and their state.
func ListSubmodules(repoPath string) ([]SubmoduleEntry, error) {
	out, err := run(repoPath, "submodule", "status")
	if err != nil {
		return nil, err
	}
	var entries []SubmoduleEntry
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if len(line) < 42 {
			continue
		}
		marker := line[0]
		rest := line[1:]
		sha := rest[:40]
		afterSha := strings.TrimPrefix(rest[40:], " ")
		path := afterSha
		if idx := strings.Index(afterSha, " ("); idx > 0 {
			path = afterSha[:idx]
		}
		e := SubmoduleEntry{Path: path, Hash: sha}
		switch marker {
		case '-':
			// not initialized
		case '+':
			e.Initialized, e.OutOfSync = true, true
		case 'U':
			e.Initialized, e.HasConflicts = true, true
		default: // ' '
			e.Initialized = true
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// SubmoduleUpdate runs `git submodule update --init --recursive`. When remote
// is true, also passes --remote (advance each submodule to its tracked
// branch's latest commit).
func SubmoduleUpdate(repoPath string, remote bool) error {
	args := []string{"submodule", "update", "--init", "--recursive"}
	if remote {
		args = append(args, "--remote")
	}
	_, err := runLocked(repoPath, args...)
	return err
}

// --- per-repo git config (local) ---

// ConfigGet reads a git config value as the repo would see it at runtime —
// system / global / local merged, local wins. Returns "" and nil error when
// the key is unset (git config --get exits 1 in that case).
func ConfigGet(repoPath, key string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "config", "--get", key)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return "", nil // key unset
		}
		return "", fmt.Errorf("git config get %s: %w", key, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ConfigSet writes a repo-local git config value.
func ConfigSet(repoPath, key, value string) error {
	_, err := runLocked(repoPath, "config", "--local", key, value)
	return err
}

// --- ahead / behind / rebase ---

// AheadBehind returns how many commits HEAD is ahead and behind upstream.
// upstream should be a ref the local repo knows, e.g. "origin/main".
func AheadBehind(repoPath, upstream string) (ahead, behind int, err error) {
	aOut, aErr := run(repoPath, "rev-list", "--count", upstream+"..HEAD")
	if aErr != nil {
		return 0, 0, aErr
	}
	bOut, bErr := run(repoPath, "rev-list", "--count", "HEAD.."+upstream)
	if bErr != nil {
		return 0, 0, bErr
	}
	ahead, _ = strconv.Atoi(strings.TrimSpace(aOut))
	behind, _ = strconv.Atoi(strings.TrimSpace(bOut))
	return ahead, behind, nil
}

// LogOneline returns one-line log entries for the given git revision range.
func LogOneline(repoPath, revRange string) ([]string, error) {
	out, err := run(repoPath, "log", "--oneline", revRange)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}

// IsDirty reports whether the working tree or index has any uncommitted changes.
func IsDirty(repoPath string) (bool, error) {
	out, err := run(repoPath, "--no-optional-locks", "status", "--porcelain", "-z")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// RebaseOnto rebases the current branch onto upstream (e.g. "origin/main").
// On success returns nil, nil. On conflict, aborts the rebase and returns the
// conflicting file paths plus a sentinel error.
func RebaseOnto(repoPath, upstream string) (conflictFiles []string, err error) {
	_, err = runLocked(repoPath, "-c", "advice.diverging=false", "rebase", upstream)
	if err == nil {
		return nil, nil
	}
	out, _ := run(repoPath, "diff", "--name-only", "--diff-filter=U")
	for _, f := range strings.Split(strings.TrimSpace(out), "\n") {
		if f != "" {
			conflictFiles = append(conflictFiles, f)
		}
	}
	_, _ = runLocked(repoPath, "rebase", "--abort")
	return conflictFiles, fmt.Errorf("rebase conflict")
}

// --- git LFS ---

// LfsAvailable reports whether the git-lfs binary is on PATH.
func LfsAvailable() bool {
	_, err := exec.LookPath("git-lfs")
	return err == nil
}

// LfsInstallLocal wires LFS hooks/filters for this repo only.
func LfsInstallLocal(repoPath string) error {
	_, err := runLocked(repoPath, "lfs", "install", "--local")
	return err
}

// LfsTrack tells git-lfs to track the given pattern (writes .gitattributes).
// `git lfs track` is idempotent for an already-tracked pattern.
func LfsTrack(repoPath, pattern string) error {
	_, err := runLocked(repoPath, "lfs", "track", pattern)
	return err
}

// StageSubmodule stages a submodule's updated gitlink in its parent repo —
// `git -C <parent> add <relPath>`. go-git's index code mishandles gitlinks,
// so this is done with the real git binary.
func StageSubmodule(parentPath, relPath string) error {
	_, err := runLocked(parentPath, "add", "--", relPath)
	return err
}
