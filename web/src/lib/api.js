// Thin fetch wrapper for the gogitit API. All requests include credentials
// so the session cookie travels for OIDC mode; harmless when auth disabled.

const BASE = '/api/v1';

async function request(path, opts = {}) {
  const res = await fetch(BASE + path, {
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(opts.headers || {}),
    },
    ...opts,
  });

  if (res.status === 401) {
    // Auth required: redirect to login flow.
    window.location.href = '/auth/login';
    throw new Error('unauthorized');
  }

  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `HTTP ${res.status}`);
  }

  if (res.status === 204) return null;
  return res.json();
}

export const api = {
  me: () => request('/me'),

  getSettings: () => request('/settings'),
  saveSettings: (payload) =>
    request('/settings', { method: 'PUT', body: JSON.stringify(payload) }),

  getMcp: () => request('/settings/mcp'),
  saveMcp: (payload) =>
    request('/settings/mcp', { method: 'PUT', body: JSON.stringify(payload) }),

  listRepos: () => request('/repos'),
  repoStatuses: () => request('/repos/statuses'),
  addRepo: (name, path) =>
    request('/repos', { method: 'POST', body: JSON.stringify({ name, path }) }),
  removeRepo: (id) => request(`/repos/${id}`, { method: 'DELETE' }),

  status: (id) => request(`/repos/${id}/status`),
  log: (id) => request(`/repos/${id}/log`),
  graph: (id) => request(`/repos/${id}/graph`),
  branches: (id) => request(`/repos/${id}/branches`),
  // createBranch at HEAD; pass hash to attach a branch to a specific commit.
  createBranch: (id, name, hash) =>
    request(`/repos/${id}/branches`, {
      method: 'POST',
      body: JSON.stringify({ name, hash }),
    }),
  deleteBranch: (id, name) =>
    request(`/repos/${id}/branches/${encodeURIComponent(name)}`, { method: 'DELETE' }),
  // switchBranch — pass stash=true to stash pending changes before checking out.
  switchBranch: (id, name, stash) =>
    request(`/repos/${id}/checkout`, {
      method: 'POST',
      body: JSON.stringify({ name, stash }),
    }),
  mergeBranch: (id, name) =>
    request(`/repos/${id}/merge`, { method: 'POST', body: JSON.stringify({ name }) }),

  // Working-tree diff for one file. staged=true → HEAD↔index, else index↔worktree.
  diff: (id, path, staged) =>
    request(`/repos/${id}/diff?path=${encodeURIComponent(path)}&staged=${staged ? '1' : '0'}`),
  // Structured diff for every file in a commit.
  commitDiff: (id, hash) => request(`/repos/${id}/commit/${hash}/diff`),

  // Index manipulation — each returns the refreshed working-tree status.
  stage: (id, paths) =>
    request(`/repos/${id}/stage`, { method: 'POST', body: JSON.stringify({ paths }) }),
  unstage: (id, paths) =>
    request(`/repos/${id}/unstage`, { method: 'POST', body: JSON.stringify({ paths }) }),
  discard: (id, paths) =>
    request(`/repos/${id}/discard`, { method: 'POST', body: JSON.stringify({ paths }) }),
  commit: (id, summary, description) =>
    request(`/repos/${id}/commit`, {
      method: 'POST',
      body: JSON.stringify({ summary, description }),
    }),

  // Remotes — add/set/remove each return the refreshed list.
  remotes: (id) => request(`/repos/${id}/remotes`),
  addRemote: (id, name, url) =>
    request(`/repos/${id}/remotes`, { method: 'POST', body: JSON.stringify({ name, url }) }),
  setRemoteUrl: (id, name, url) =>
    request(`/repos/${id}/remotes/${encodeURIComponent(name)}`, {
      method: 'PUT',
      body: JSON.stringify({ url }),
    }),
  removeRemote: (id, name) =>
    request(`/repos/${id}/remotes/${encodeURIComponent(name)}`, { method: 'DELETE' }),

  // Transport — each returns { remote, status }.
  fetchRemote: (id) => request(`/repos/${id}/fetch`, { method: 'POST' }),
  pull: (id) => request(`/repos/${id}/pull`, { method: 'POST' }),
  push: (id) => request(`/repos/${id}/push`, { method: 'POST' }),

  // Stash (system git) — stash/pop return { count, status }.
  stashList: (id) => request(`/repos/${id}/stash`),
  stash: (id) => request(`/repos/${id}/stash`, { method: 'POST' }),
  stashPop: (id) => request(`/repos/${id}/stash/pop`, { method: 'POST' }),

  // Submodule: stage + commit + push the parent's gitlink. parentUpdate is
  // invoked from a submodule's push toast (the child knows its own id).
  // submoduleCommitPush is invoked from the parent's diff view (the parent
  // knows the submodule path); the submodule does not need to be registered.
  parentUpdate: (id) => request(`/repos/${id}/parent-update`, { method: 'POST' }),
  submoduleCommitPush: (id, path) =>
    request(`/repos/${id}/submodule-commit-push`, {
      method: 'POST',
      body: JSON.stringify({ path }),
    }),

  // Per-repo LFS state (auto-tracking + git-lfs hooks).
  getLfs: (id) => request(`/repos/${id}/lfs`),
  setLfs: (id, enabled, thresholdBytes) =>
    request(`/repos/${id}/lfs`, {
      method: 'PUT',
      body: JSON.stringify({ enabled, threshold_bytes: thresholdBytes }),
    }),

  // List a repo's submodules + their state (augmented with registered_id /
  // registered_name when the submodule is already added to GoGitIt).
  submodules: (id) => request(`/repos/${id}/submodules`),
  // Run `git submodule update --init --recursive [--remote]`.
  submodulesUpdate: (id, remote) =>
    request(`/repos/${id}/submodules/update`, {
      method: 'POST',
      body: JSON.stringify({ remote: !!remote }),
    }),
};
