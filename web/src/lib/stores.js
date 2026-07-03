import { writable, derived, readable } from 'svelte/store';
import { api } from './api.js';

export const user = writable(null);

// True on narrow viewports (< md, 768px) — drives the single-column mobile
// layout: full-width sidebar drawer and master-detail drill-down.
export const isMobile = readable(false, (set) => {
  if (typeof window === 'undefined' || !window.matchMedia) return;
  const mq = window.matchMedia('(max-width: 767px)');
  set(mq.matches);
  const handler = (e) => set(e.matches);
  mq.addEventListener('change', handler);
  return () => mq.removeEventListener('change', handler);
});
export const repos = writable([]);
export const selectedRepoId = writable(null);
export const activeTab = writable('changes'); // 'changes' | 'history'

// Whether the settings modal is open; toggled by the TopBar cog.
export const settingsOpen = writable(false);

// Diff rendering mode, shared across Changes and History and persisted so the
// user's choice survives reloads. 'inline' = unified, 'split' = side-by-side.
const DIFF_MODE_KEY = 'gogitit:diffMode';
function initialDiffMode() {
  try {
    const v = localStorage.getItem(DIFF_MODE_KEY);
    return v === 'split' || v === 'inline' ? v : 'inline';
  } catch {
    return 'inline';
  }
}
export const diffMode = writable(initialDiffMode());
diffMode.subscribe((v) => {
  try {
    localStorage.setItem(DIFF_MODE_KEY, v);
  } catch {
    /* localStorage unavailable — non-fatal */
  }
});

// Whether the Repositories sidebar is expanded; toggled by the TopBar hamburger.
const SIDEBAR_KEY = 'gogitit:sidebarOpen';
function initialSidebarOpen() {
  try {
    const v = localStorage.getItem(SIDEBAR_KEY);
    return v === null ? true : v === '1';
  } catch {
    return true;
  }
}
export const sidebarOpen = writable(initialSidebarOpen());
sidebarOpen.subscribe((v) => {
  try {
    localStorage.setItem(SIDEBAR_KEY, v ? '1' : '0');
  } catch {
    /* localStorage unavailable — non-fatal */
  }
});

export const selectedRepo = derived(
  [repos, selectedRepoId],
  ([$repos, $id]) => $repos.find((r) => r.id === $id) || null
);

export async function loadMe() {
  try {
    const u = await api.me();
    user.set(u);
  } catch (e) {
    user.set(null);
  }
}

export async function loadRepos() {
  const list = await api.listRepos();
  const sorted = (list || []).slice().sort((a, b) =>
    (a.name || '').localeCompare(b.name || '', undefined, { sensitivity: 'base' })
  );
  repos.set(sorted);
}

// Divergence dialog — set when a pull detects that the branch is both ahead
// and behind its upstream. Shape: { repoId, repoName, ahead, behind,
// local_commits, remote_commits, dirty } | null
export const divergenceDialog = writable(null);

// MCP state — driven from app settings. Loaded on app start + after a save.
export const mcpState = writable(null);
export async function loadMcp() {
  try {
    mcpState.set(await api.getMcp());
  } catch {
    mcpState.set(null);
  }
}
