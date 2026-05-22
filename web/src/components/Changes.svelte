<script>
  import { createEventDispatcher } from 'svelte';
  import { api } from '../lib/api.js';
  import { activeTab, isMobile } from '../lib/stores.js';
  import { addToast } from '../lib/toasts.js';
  import { runSync } from '../lib/sync.js';
  import { setRepoStatus, summarize } from '../lib/repostatus.js';
  import { liveSignal } from '../lib/live.js';
  import DiffViewer from './DiffViewer.svelte';
  import DiffModeToggle from './DiffModeToggle.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';

  export let repo;

  const dispatch = createEventDispatcher();

  let status = null;
  let loading = true;
  let error = null;
  let working = false;
  let opError = null;
  let stashCount = 0;
  let recoverBranch = ''; // detached-HEAD recovery branch name

  // Currently inspected file: { path, staged }.
  let selectedFile = null;
  let mobileView = 'list'; // mobile drill-down: 'list' | 'diff'
  let fileDiff = null;
  let diffLoading = false;
  let diffError = null;

  let lastRepoId = null;

  async function load() {
    const id = repo.id;
    loading = true;
    error = null;
    try {
      status = await api.status(id);
      setRepoStatus(id, summarize(status));
      try {
        stashCount = (await api.stashList(id)).count;
      } catch {
        stashCount = 0; // git unavailable — stash simply disabled
      }
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function doStash() {
    working = true;
    opError = null;
    try {
      const res = await api.stash(repo.id);
      status = res.status;
      stashCount = res.count;
      setRepoStatus(repo.id, summarize(status));
      selectedFile = null;
      fileDiff = null;
      addToast({ kind: 'success', message: 'Stashed all changes' });
    } catch (e) {
      opError = e.message;
    } finally {
      working = false;
    }
  }

  async function doStashPop() {
    working = true;
    opError = null;
    try {
      const res = await api.stashPop(repo.id);
      status = res.status;
      stashCount = res.count;
      setRepoStatus(repo.id, summarize(status));
      if (selectedFile) await refreshDiff();
      addToast({ kind: 'success', message: 'Popped latest stash' });
    } catch (e) {
      opError = e.message;
    } finally {
      working = false;
    }
  }

  // Detached-HEAD recovery — create a branch at the current commit so the
  // work belongs to a branch.
  async function recoverDetached() {
    const name = recoverBranch.trim();
    if (!name) return;
    working = true;
    opError = null;
    try {
      await api.createBranch(repo.id, name);
      recoverBranch = '';
      addToast({ kind: 'success', message: `Moved work to new branch ${name}` });
      dispatch('changed'); // RepoView reloads branches + remounts this view

    } catch (e) {
      opError = e.message;
    } finally {
      working = false;
    }
  }

  async function selectFile(path, staged) {
    selectedFile = { path, staged };
    if ($isMobile) mobileView = 'diff';
    diffLoading = true;
    diffError = null;
    fileDiff = null;
    try {
      fileDiff = await api.diff(repo.id, path, staged);
    } catch (e) {
      diffError = e.message;
    } finally {
      diffLoading = false;
    }
  }

  async function refreshDiff() {
    if (!selectedFile) return;
    try {
      fileDiff = await api.diff(repo.id, selectedFile.path, selectedFile.staged);
      diffError = null;
    } catch (e) {
      diffError = e.message;
    }
  }

  // Apply a stage/unstage/discard op. The endpoint returns the fresh status.
  async function runOp(kind, paths) {
    if (!paths.length) return;
    working = true;
    opError = null;
    try {
      if (kind === 'stage') status = await api.stage(repo.id, paths);
      else if (kind === 'unstage') status = await api.unstage(repo.id, paths);
      else status = await api.discard(repo.id, paths);
      setRepoStatus(repo.id, summarize(status));

      // Keep the diff pane consistent with where the file landed.
      if (selectedFile && paths.includes(selectedFile.path)) {
        if (kind === 'discard') {
          selectedFile = null;
          fileDiff = null;
        } else {
          selectedFile = { ...selectedFile, staged: kind === 'stage' };
          await refreshDiff();
        }
      } else if (selectedFile) {
        await refreshDiff();
      }
    } catch (e) {
      opError = e.message;
    } finally {
      working = false;
    }
  }

  // In-app confirmation dialog, parametrised per action.
  let confirmState = null; // { title, message, confirmLabel, danger, onConfirm }

  function askConfirm(opts) {
    confirmState = opts;
  }
  function resolveConfirm() {
    const cb = confirmState?.onConfirm;
    confirmState = null;
    cb?.();
  }

  function confirmDiscard(paths) {
    askConfirm({
      title: 'Discard changes',
      message:
        paths.length === 1
          ? `Discard all changes to "${paths[0]}"?\n` +
            'The file is reverted to HEAD (a new file is deleted). This cannot be undone.'
          : `Discard changes to ${paths.length} files?\nThis cannot be undone.`,
      confirmLabel: 'Discard',
      danger: true,
      onConfirm: () => runOp('discard', paths),
    });
  }

  // --- Commit ---
  let summary = '';
  let description = '';

  $: canCommit =
    !working && summary.trim() !== '' && (status?.staged?.length || 0) > 0;

  function attemptCommit() {
    if (!canCommit) return;
    if (status?.detached) {
      askConfirm({
        title: 'Commit on a detached HEAD',
        message:
          'HEAD is detached — this commit will not belong to any branch and ' +
          'can be lost once you switch away.\n\n' +
          'Consider switching to a branch first. Commit here anyway?',
        confirmLabel: 'Commit anyway',
        danger: true,
        onConfirm: doCommit,
      });
    } else {
      doCommit();
    }
  }

  async function doCommit() {
    working = true;
    opError = null;
    try {
      const res = await api.commit(repo.id, summary.trim(), description.trim());
      status = res.status;
      setRepoStatus(repo.id, summarize(status));
      summary = '';
      description = '';
      selectedFile = null;
      fileDiff = null;

      // Toast the new commit; offer Push when there is something to push.
      const ahead = res.status.ahead || 0;
      if (ahead > 0) {
        addToast({
          kind: 'success',
          message: `Committed ${res.short_hash} on ${repo.name} — ${ahead} commit${ahead === 1 ? '' : 's'} to push`,
          action: { label: 'Push', run: () => runSync(repo.id, repo.name, 'push') },
          timeout: 0,
        });
      } else {
        addToast({ kind: 'success', message: `Committed ${res.short_hash} on ${repo.name}` });
      }

      // Working tree clean — jump to History to show the new commit.
      const remaining =
        res.status.staged.length +
        res.status.unstaged.length +
        res.status.untracked.length;
      if (remaining === 0) activeTab.set('history');
    } catch (e) {
      opError = e.message;
    } finally {
      working = false;
    }
  }

  function isSelected(path, staged) {
    return selectedFile && selectedFile.path === path && selectedFile.staged === staged;
  }

  // Reset the diff pane only when the repo actually changes, so a manual
  // Refresh or an index op keeps the file the user was looking at.
  $: if (repo && repo.id !== lastRepoId) {
    lastRepoId = repo.id;
    selectedFile = null;
    fileDiff = null;
    diffError = null;
    opError = null;
    load();
  }

  // A live (WebSocket) update for this repo — soft-refresh the file list
  // without the loading flicker, keeping the commit draft and selection.
  let lastLiveTs = 0;
  async function liveReload() {
    try {
      status = await api.status(repo.id);
      setRepoStatus(repo.id, summarize(status));
      if (selectedFile) await refreshDiff();
    } catch (e) {
      /* ignore — manual Refresh still works */
    }
  }
  $: if ($liveSignal && repo && $liveSignal.repoId === repo.id && $liveSignal.ts !== lastLiveTs) {
    lastLiveTs = $liveSignal.ts;
    liveReload();
  }

  $: changeCount = status
    ? status.staged.length + status.unstaged.length + status.untracked.length
    : 0;

  // Mobile: snap back to the file list whenever the inspected file clears.
  $: if ($isMobile && !selectedFile) mobileView = 'list';

  function statusBadge(s) {
    const map = { M: 'Modified', A: 'Added', D: 'Deleted', R: 'Renamed', '?': 'Untracked' };
    return map[s] || s;
  }
  function statusColor(s) {
    if (s === 'A') return 'text-success';
    if (s === 'D') return 'text-danger';
    if (s === '?') return 'text-attention';
    return 'text-accent';
  }
</script>

<div class="h-full flex">
  <!-- File list -->
  <div
    class="w-full md:w-80 md:shrink-0 min-w-0 border-r border-border flex-col overflow-hidden
           {mobileView === 'list' ? 'flex' : 'hidden'} md:flex"
  >
    <div class="px-3 py-2 border-b border-border flex items-center justify-between">
      <span class="text-xs uppercase tracking-wider text-fg-muted font-semibold">Changes</span>
      <button class="text-xs text-fg-muted hover:text-fg" on:click={load}>Refresh</button>
    </div>

    {#if opError}
      <p class="px-3 py-2 text-xs text-danger bg-danger/10 border-b border-border">{opError}</p>
    {/if}

    <div class="flex-1 overflow-y-auto">
      {#if loading}
        <p class="px-3 py-4 text-sm text-fg-muted">Loading…</p>
      {:else if error}
        <p class="px-3 py-4 text-sm text-danger">{error}</p>
      {:else if changeCount === 0}
        <p class="px-3 py-4 text-sm text-fg-muted">No changes.</p>
      {:else}
        {#if status.staged.length}
          <div class="px-3 pt-3 pb-1 flex items-center justify-between">
            <span class="text-xs text-fg-muted uppercase tracking-wider">Staged</span>
            <button
              class="text-xs text-fg-muted hover:text-fg disabled:opacity-40"
              disabled={working}
              on:click={() => runOp('unstage', status.staged.map((f) => f.path))}
            >Unstage all</button>
          </div>
          <ul>
            {#each status.staged as f (f.path)}
              <li
                class="flex items-center group hover:bg-border-muted transition-colors
                       {isSelected(f.path, true) ? 'bg-accent-subtle' : ''}"
              >
                <button
                  class="flex-1 min-w-0 px-3 py-1.5 flex items-center gap-2 text-left"
                  title={statusBadge(f.status)}
                  on:click={() => selectFile(f.path, true)}
                >
                  <span class="font-mono text-xs {statusColor(f.status)}">{f.status}</span>
                  <span class="truncate text-sm">{f.path}</span>
                </button>
                <div class="flex items-center gap-1 pr-2 shrink-0
                            opacity-0 group-hover:opacity-100 transition-opacity">
                  <button class="btn-mini" disabled={working}
                          on:click={() => runOp('unstage', [f.path])}>Unstage</button>
                </div>
              </li>
            {/each}
          </ul>
        {/if}

        {#if status.unstaged.length}
          <div class="px-3 pt-3 pb-1 flex items-center justify-between">
            <span class="text-xs text-fg-muted uppercase tracking-wider">Unstaged</span>
            <button
              class="text-xs text-fg-muted hover:text-fg disabled:opacity-40"
              disabled={working}
              on:click={() => runOp('stage', status.unstaged.map((f) => f.path))}
            >Stage all</button>
          </div>
          <ul>
            {#each status.unstaged as f (f.path)}
              <li
                class="flex items-center group hover:bg-border-muted transition-colors
                       {isSelected(f.path, false) ? 'bg-accent-subtle' : ''}"
              >
                <button
                  class="flex-1 min-w-0 px-3 py-1.5 flex items-center gap-2 text-left"
                  title={statusBadge(f.status)}
                  on:click={() => selectFile(f.path, false)}
                >
                  <span class="font-mono text-xs {statusColor(f.status)}">{f.status}</span>
                  <span class="truncate text-sm">{f.path}</span>
                </button>
                <div class="flex items-center gap-1 pr-2 shrink-0
                            opacity-0 group-hover:opacity-100 transition-opacity">
                  <button class="btn-mini" disabled={working}
                          on:click={() => runOp('stage', [f.path])}>Stage</button>
                  <button class="btn-mini hover:!text-danger" disabled={working}
                          on:click={() => confirmDiscard([f.path])}>Discard</button>
                </div>
              </li>
            {/each}
          </ul>
        {/if}

        {#if status.untracked.length}
          <div class="px-3 pt-3 pb-1 flex items-center justify-between">
            <span class="text-xs text-fg-muted uppercase tracking-wider">Untracked</span>
            <button
              class="text-xs text-fg-muted hover:text-fg disabled:opacity-40"
              disabled={working}
              on:click={() => runOp('stage', [...status.untracked])}
            >Stage all</button>
          </div>
          <ul>
            {#each status.untracked as p (p)}
              <li
                class="flex items-center group hover:bg-border-muted transition-colors
                       {isSelected(p, false) ? 'bg-accent-subtle' : ''}"
              >
                <button
                  class="flex-1 min-w-0 px-3 py-1.5 flex items-center gap-2 text-left"
                  title="Untracked"
                  on:click={() => selectFile(p, false)}
                >
                  <span class="font-mono text-xs text-attention">?</span>
                  <span class="truncate text-sm">{p}</span>
                </button>
                <div class="flex items-center gap-1 pr-2 shrink-0
                            opacity-0 group-hover:opacity-100 transition-opacity">
                  <button class="btn-mini" disabled={working}
                          on:click={() => runOp('stage', [p])}>Stage</button>
                  <button class="btn-mini hover:!text-danger" disabled={working}
                          on:click={() => confirmDiscard([p])}>Discard</button>
                </div>
              </li>
            {/each}
          </ul>
        {/if}
      {/if}
    </div>

    <!-- Commit area -->
    <div class="border-t border-border p-3 space-y-2">
      <div class="flex gap-2">
        <button class="btn-mini flex-1" disabled={working || changeCount === 0}
                on:click={doStash}>Stash all</button>
        <button class="btn-mini flex-1" disabled={working || stashCount === 0}
                on:click={doStashPop}>Pop stash{stashCount > 0 ? ` (${stashCount})` : ''}</button>
      </div>

      {#if status?.detached}
        <div class="rounded border border-attention/40 bg-attention/10 p-2 space-y-1.5">
          <p class="text-[11px] text-attention">
            Detached HEAD — create a branch to keep your work on a branch.
          </p>
          <div class="flex gap-1.5">
            <input
              class="input"
              placeholder="new branch name"
              bind:value={recoverBranch}
              disabled={working}
              on:keydown={(e) => e.key === 'Enter' && recoverDetached()}
            />
            <button
              class="btn btn-primary shrink-0 justify-center"
              disabled={working || !recoverBranch.trim()}
              on:click={recoverDetached}
            >Create</button>
          </div>
        </div>
      {/if}

      <input
        class="input"
        placeholder="Summary (required)"
        bind:value={summary}
        disabled={working}
      />
      <textarea
        class="input resize-none"
        rows="2"
        placeholder="Description"
        bind:value={description}
        disabled={working}
      ></textarea>
      <button
        class="btn btn-primary w-full justify-center"
        disabled={!canCommit}
        on:click={attemptCommit}
      >
        {#if working}Committing…
        {:else if status?.detached}Commit (detached HEAD)
        {:else}Commit to {status?.branch || '—'}
        {/if}
      </button>
    </div>
  </div>

  <!-- Diff pane -->
  <div
    class="flex-1 min-w-0 flex-col overflow-hidden
           {mobileView === 'diff' ? 'flex' : 'hidden'} md:flex"
  >
    {#if selectedFile}
      <div class="px-3 py-2 border-b border-border flex items-center justify-between gap-3">
        <div class="flex items-center gap-1.5 min-w-0">
          <button
            class="md:hidden shrink-0 text-accent text-lg leading-none px-1 -ml-1"
            aria-label="Back to changes"
            on:click={() => (mobileView = 'list')}
          >‹</button>
          <span class="text-sm font-mono text-fg truncate">{selectedFile.path}</span>
        </div>
        <div class="flex items-center gap-2 shrink-0">
          <span class="text-xs text-fg-muted">{selectedFile.staged ? 'Staged' : 'Working tree'}</span>
          <DiffModeToggle />
        </div>
      </div>
      <div class="flex-1 overflow-y-auto">
        {#if diffLoading}
          <p class="px-3 py-4 text-sm text-fg-muted">Loading diff…</p>
        {:else if diffError}
          <p class="px-3 py-4 text-sm text-danger">{diffError}</p>
        {:else if fileDiff}
          <DiffViewer file={fileDiff} />
        {/if}
      </div>
    {:else}
      <div class="flex-1 flex items-center justify-center text-fg-muted text-sm">
        Select a file to see the diff
      </div>
    {/if}
  </div>
</div>

<ConfirmDialog
  open={!!confirmState}
  title={confirmState?.title || ''}
  message={confirmState?.message || ''}
  confirmLabel={confirmState?.confirmLabel || 'Confirm'}
  danger={confirmState?.danger || false}
  on:confirm={resolveConfirm}
  on:cancel={() => (confirmState = null)}
/>
