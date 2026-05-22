// Per-repo status summaries that drive the sidebar indicators. Kept app-level
// so every repo row reflects state, not just the selected one.

import { writable } from 'svelte/store';
import { api } from './api.js';

// { [repoId]: { id, branch, detached, ahead, behind, changed_files, error } }
export const repoStatuses = writable({});

// refreshRepoStatuses reloads the summary for every registered repo.
export async function refreshRepoStatuses() {
  try {
    const list = await api.repoStatuses();
    const map = {};
    for (const s of list || []) map[s.id] = s;
    repoStatuses.set(map);
  } catch {
    /* leave stale — non-fatal */
  }
}

// setRepoStatus updates one repo's entry from a status already in hand,
// avoiding a refetch when a component just loaded that repo's full status.
export function setRepoStatus(id, summary) {
  repoStatuses.update((m) => ({ ...m, [id]: { ...summary, id } }));
}

// summarize condenses a full working-tree status into the sidebar summary.
export function summarize(st) {
  return {
    branch: st.branch,
    detached: !!st.detached,
    local_branch: !!st.local_branch,
    ahead: st.ahead || 0,
    behind: st.behind || 0,
    changed_files:
      (st.staged?.length || 0) +
      (st.unstaged?.length || 0) +
      (st.untracked?.length || 0),
  };
}
