// App-level fetch / pull / push. Lives outside components so an operation
// keeps running — and still reports via a toast — after the user switches to
// another repo. Start a fetch on a big repo, go work elsewhere, and act on the
// result straight from the toast.

import { writable, get } from 'svelte/store';
import { api } from './api.js';
import { repos, divergenceDialog } from './stores.js';
import { addToast } from './toasts.js';
import { refreshRepoStatuses } from './repostatus.js';

// In-flight ops, keyed by repo id: { [repoId]: 'fetch' | 'pull' | 'push' }.
export const syncing = writable({});

// Outcome of the last op per repo: { [repoId]: 'error' | 'merge' }. Cleared on
// the next successful op. Drives the sidebar error / merge-attention icons.
export const lastResult = writable({});

// Last completed op — RepoView watches this to refresh the matching repo view.
export const syncSignal = writable(null);

function setSyncing(repoId, kind) {
  syncing.update((s) => ({ ...s, [repoId]: kind }));
}
function clearSyncing(repoId) {
  syncing.update((s) => {
    const next = { ...s };
    delete next[repoId];
    return next;
  });
}
function setResult(repoId, value) {
  lastResult.update((m) => {
    const next = { ...m };
    if (value) next[repoId] = value;
    else delete next[repoId];
    return next;
  });
}

const VERB = { fetch: 'Fetched', pull: 'Pulled', push: 'Pushed' };

function plural(n) {
  return n === 1 ? '' : 's';
}

// hasParent reports whether a repo is a submodule of another registered repo.
function hasParent(repoId) {
  const r = get(repos).find((x) => x.id === repoId);
  return !!(r && r.parent_id);
}

// runSync runs a transport op for a specific repo and reports it via a toast.
// Actionable toasts chain the natural next step: fetch → pull, pull → push.
export async function runSync(repoId, repoName, kind) {
  setSyncing(repoId, kind);
  try {
    const call =
      kind === 'fetch' ? api.fetchRemote : kind === 'pull' ? api.pull : api.push;
    const res = await call(repoId);

    if (kind === 'pull' && res && res.diverged) {
      divergenceDialog.set({ repoId, repoName, ...res });
      return;
    }

    const st = res.status || {};

    if (kind === 'fetch' && (st.behind || 0) > 0) {
      addToast({
        kind: 'success',
        message: `Fetched ${repoName} — ${st.behind} new commit${plural(st.behind)} to pull`,
        action: { label: 'Pull', run: () => runSync(repoId, repoName, 'pull') },
        timeout: 0,
      });
    } else if (kind === 'pull' && (st.ahead || 0) > 0) {
      addToast({
        kind: 'success',
        message: `Pulled ${repoName} — ${st.ahead} commit${plural(st.ahead)} to push`,
        action: { label: 'Push', run: () => runSync(repoId, repoName, 'push') },
        timeout: 0,
      });
    } else if (kind === 'push' && hasParent(repoId)) {
      // A submodule was pushed — its parent's gitlink is now stale.
      addToast({
        kind: 'success',
        message: `Pushed ${repoName} to ${res.remote}`,
        action: {
          label: 'Update parent reference & push',
          run: () => runParentUpdate(repoId, repoName),
        },
        timeout: 0,
      });
    } else {
      const tail =
        kind === 'fetch' ? ' — up to date'
        : kind === 'push' ? ` to ${res.remote}`
        : '';
      addToast({ kind: 'success', message: `${VERB[kind]} ${repoName}${tail}` });
    }

    setResult(repoId, null);
    syncSignal.set({ repoId, kind, ts: Date.now() });
    refreshRepoStatuses();
  } catch (e) {
    // A non-fast-forward pull needs a manual merge — flag it distinctly.
    const merge = /fast-forward|merge/i.test(e.message);
    setResult(repoId, merge ? 'merge' : 'error');
    addToast({
      kind: 'error',
      message: `${VERB[kind] || kind} failed — ${repoName}: ${e.message}`,
      timeout: 9000,
    });
  } finally {
    clearSyncing(repoId);
  }
}

// runRebasePush resolves a diverged branch: rebase onto upstream + push.
// Called from DivergeDialog after the user confirms.
export async function runRebasePush(repoId, repoName) {
  divergenceDialog.set(null);
  setSyncing(repoId, 'pull');
  try {
    await api.rebasePush(repoId);
    addToast({ kind: 'success', message: `Rebased and pushed ${repoName}` });
    setResult(repoId, null);
    syncSignal.set({ repoId, kind: 'pull', ts: Date.now() });
    refreshRepoStatuses();
  } catch (e) {
    setResult(repoId, 'error');
    addToast({
      kind: 'error',
      message: `Rebase & Push failed — ${repoName}: ${e.message}`,
      timeout: 0,
    });
  } finally {
    clearSyncing(repoId);
  }
}

// runParentUpdate, for a submodule repo, commits its updated gitlink in the
// parent repo and pushes the parent.
export async function runParentUpdate(repoId, repoName) {
  setSyncing(repoId, 'push');
  try {
    const res = await api.parentUpdate(repoId);
    addToast({
      kind: 'success',
      message: `Updated ${res.parent} — submodule reference committed and pushed`,
    });
    const parentId = get(repos).find((r) => r.id === repoId)?.parent_id;
    if (parentId) {
      syncSignal.set({ repoId: parentId, kind: 'parent-update', ts: Date.now() });
    }
    refreshRepoStatuses();
  } catch (e) {
    addToast({
      kind: 'error',
      message: `Parent update failed — ${repoName}: ${e.message}`,
      timeout: 9000,
    });
  } finally {
    clearSyncing(repoId);
  }
}
