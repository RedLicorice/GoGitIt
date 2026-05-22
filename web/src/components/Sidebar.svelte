<script>
  import { repos, selectedRepoId, loadRepos, isMobile, sidebarOpen } from '../lib/stores.js';
  import { api } from '../lib/api.js';
  import { syncing, lastResult } from '../lib/sync.js';
  import { repoStatuses } from '../lib/repostatus.js';
  import RepoStatusIcon from './RepoStatusIcon.svelte';

  // Priority cascade for a repo's row indicator: running op > last-op error >
  // diverged/merge > uncommitted changes > unpushed > behind > clean.
  // The store maps are passed in (not read via `$` inside) so the markup
  // expression depends on them and re-renders when they update.
  function repoIndicator(id, syncing, lastResult, statuses) {
    const op = syncing[id];
    if (op) return { kind: 'op', op };
    const res = lastResult[id];
    if (res === 'error') return { kind: 'error' };
    if (res === 'merge') return { kind: 'merge' };
    const st = statuses[id];
    if (!st) return null;
    if (st.error) return { kind: 'error' };
    if (st.ahead > 0 && st.behind > 0) return { kind: 'merge' };
    if (st.changed_files > 0) return { kind: 'changes', n: st.changed_files };
    if (st.ahead > 0) return { kind: 'ahead', n: st.ahead };
    if (st.behind > 0) return { kind: 'behind', n: st.behind };
    return { kind: 'clean' };
  }

  let showAdd = false;
  let newPath = '';
  let newName = '';
  let adding = false;
  let addError = null;

  async function handleAdd() {
    adding = true;
    addError = null;
    try {
      await api.addRepo(newName, newPath);
      await loadRepos();
      newPath = '';
      newName = '';
      showAdd = false;
    } catch (e) {
      addError = e.message;
    } finally {
      adding = false;
    }
  }

  function selectRepo(id) {
    selectedRepoId.set(id);
    if ($isMobile) sidebarOpen.set(false); // close the drawer after picking
  }

  async function handleRemove(id) {
    if (!confirm('Remove this repository from GoGitIt? (Files on disk are not deleted)')) return;
    await api.removeRepo(id);
    if ($selectedRepoId === id) selectedRepoId.set(null);
    await loadRepos();
  }
</script>

<aside
  class="flex flex-col border-r border-border bg-canvas-subtle
         fixed inset-x-0 top-12 bottom-0 z-30 w-full
         md:static md:inset-auto md:z-auto md:w-64 md:shrink-0"
>
  <div class="px-3 py-2 flex items-center justify-between border-b border-border">
    <span class="text-xs uppercase tracking-wider text-fg-muted font-semibold">
      Repositories
    </span>
    <button
      class="text-fg-muted hover:text-fg transition-colors text-lg leading-none"
      title="Add repository"
      on:click={() => (showAdd = !showAdd)}
    >+</button>
  </div>

  {#if showAdd}
    <div class="p-3 border-b border-border space-y-2">
      <input class="input" placeholder="Name (optional)" bind:value={newName} />
      <input class="input" placeholder="/absolute/path/to/repo" bind:value={newPath} />
      {#if addError}
        <p class="text-xs text-danger">{addError}</p>
      {/if}
      <div class="flex gap-2">
        <button class="btn btn-primary flex-1 justify-center" disabled={adding || !newPath}
                on:click={handleAdd}>
          {adding ? 'Adding…' : 'Add'}
        </button>
        <button class="btn" on:click={() => (showAdd = false)}>Cancel</button>
      </div>
    </div>
  {/if}

  <div class="flex-1 overflow-y-auto">
    {#if $repos.length === 0}
      <p class="px-3 py-4 text-sm text-fg-muted">
        No repositories yet. Click + to add one.
      </p>
    {:else}
      <ul>
        {#each $repos as r (r.id)}
          <li
            class="flex items-stretch group hover:bg-border-muted transition-colors
                   {$selectedRepoId === r.id ? 'bg-accent-subtle border-l-2 border-accent' : ''}"
          >
            <button
              class="flex-1 min-w-0 text-left px-3 py-2"
              on:click={() => selectRepo(r.id)}
            >
              <div class="text-sm text-fg truncate">{r.name}</div>
              <div class="text-xs text-fg-muted truncate">{r.path}</div>
              {#if r.parent_name}
                <div
                  class="mt-0.5 flex items-center gap-1 text-xs text-fg-muted"
                  title="Submodule of {r.parent_name}"
                >
                  <svg viewBox="0 0 16 16" class="w-3 h-3 shrink-0" fill="currentColor" aria-hidden="true">
                    <path d="M5 5.372v.878c0 .414.336.75.75.75h4.5a.75.75 0 0 0 .75-.75v-.878a2.25 2.25 0 1 1 1.5 0v.878a2.25 2.25 0 0 1-2.25 2.25h-1.5v2.128a2.251 2.251 0 1 1-1.5 0V8.5h-1.5A2.25 2.25 0 0 1 3.5 6.25v-.878a2.25 2.25 0 1 1 1.5 0ZM5 3.25a.75.75 0 1 0-1.5 0 .75.75 0 0 0 1.5 0Zm6.75.75a.75.75 0 1 0 0-1.5.75.75 0 0 0 0 1.5Zm-3 8.75a.75.75 0 1 0-1.5 0 .75.75 0 0 0 1.5 0Z"/>
                  </svg>
                  <span class="truncate">{r.parent_name}</span>
                </div>
              {/if}
            </button>
            <RepoStatusIcon
              state={repoIndicator(r.id, $syncing, $lastResult, $repoStatuses)}
            />
            <button
              class="px-3 shrink-0 text-sm text-fg-muted hover:text-danger
                     opacity-0 group-hover:opacity-100 transition-opacity"
              title="Remove repository"
              aria-label="Remove repository"
              on:click={() => handleRemove(r.id)}
            >×</button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</aside>
