<script>
  import { activeTab } from '../lib/stores.js';
  import { api } from '../lib/api.js';
  import { runSync, syncing, syncSignal } from '../lib/sync.js';
  import { repoStatuses, refreshRepoStatuses } from '../lib/repostatus.js';
  import Changes from './Changes.svelte';
  import History from './History.svelte';
  import RepoSettings from './RepoSettings.svelte';
  import BranchMenu from './BranchMenu.svelte';

  export let repo;

  let branches = [];
  let currentBranch = '';
  // Bumped to remount the active view so it reloads after a sync.
  let dataVersion = 0;
  let lastRepoId = null;
  let lastSignalTs = 0;

  async function loadBranches() {
    try {
      branches = await api.branches(repo.id);
      currentBranch = (branches.find((b) => b.is_current) || {}).name || '';
    } catch (e) {
      branches = [];
    }
  }

  // Whatever op is in flight for this repo (undefined when idle).
  $: busy = $syncing[repo?.id];

  // Ahead/behind from the app-level status store; diverged = both nonzero.
  $: st = $repoStatuses[repo?.id];
  $: diverged = !!(st && st.ahead > 0 && st.behind > 0);

  // Refresh the view when a sync for this repo finishes (also covers a sync
  // started from a toast for a repo the user has since navigated back to).
  $: if ($syncSignal && $syncSignal.repoId === repo?.id && $syncSignal.ts !== lastSignalTs) {
    lastSignalTs = $syncSignal.ts;
    loadBranches();
    dataVersion += 1;
  }

  $: if (repo && repo.id !== lastRepoId) {
    lastRepoId = repo.id;
    loadBranches();
  }

  // A branch create/switch/delete/merge changed the repo — refresh everything.
  function onBranchChanged() {
    loadBranches();
    dataVersion += 1;
    refreshRepoStatuses();
  }

</script>

<div class="flex-1 flex flex-col overflow-hidden">
  <!-- Tab bar + sync actions + branch indicator -->
  <div
    class="h-11 shrink-0 flex items-center border-b border-border bg-canvas-subtle
           px-2 overflow-x-auto"
  >
    <nav class="flex shrink-0">
      <button
        class="px-4 py-2 text-sm font-medium border-b-2 transition-colors
               {$activeTab === 'changes'
                 ? 'border-accent text-fg'
                 : 'border-transparent text-fg-muted hover:text-fg'}"
        on:click={() => activeTab.set('changes')}
      >Changes</button>
      <button
        class="px-4 py-2 text-sm font-medium border-b-2 transition-colors
               {$activeTab === 'history'
                 ? 'border-accent text-fg'
                 : 'border-transparent text-fg-muted hover:text-fg'}"
        on:click={() => activeTab.set('history')}
      >History</button>
      <button
        class="px-4 py-2 text-sm font-medium border-b-2 transition-colors
               {$activeTab === 'settings'
                 ? 'border-accent text-fg'
                 : 'border-transparent text-fg-muted hover:text-fg'}"
        on:click={() => activeTab.set('settings')}
      >Settings</button>
    </nav>

    <div class="ml-auto flex items-center gap-1 pr-2 shrink-0">
      <!-- Fetch -->
      <button
        class="p-1 rounded transition-colors disabled:cursor-default
               {busy === 'fetch'
                 ? 'text-fg-muted animate-pulse'
                 : busy
                   ? 'text-fg-muted opacity-40'
                   : 'text-fg-muted hover:text-fg hover:bg-border-muted'}"
        disabled={!!busy}
        title={busy === 'fetch' ? 'Fetching… (in progress)' : 'Fetch'}
        aria-label="Fetch"
        on:click={() => runSync(repo.id, repo.name, 'fetch')}
      >
        <svg viewBox="0 0 16 16" class="w-4 h-4" fill="none" stroke="currentColor"
             stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M13.5 8A5.5 5.5 0 1 1 11.9 4.1"/>
          <path d="M12 1.5V4.5H9"/>
        </svg>
      </button>
      <!-- Pull -->
      <button
        class="p-1 rounded transition-colors disabled:cursor-default
               {busy === 'pull'
                 ? 'text-accent animate-pulse'
                 : busy
                   ? 'text-accent opacity-40'
                   : 'text-accent hover:bg-border-muted'}"
        disabled={!!busy}
        title={busy === 'pull' ? 'Pulling… (in progress)' : 'Pull'}
        aria-label="Pull"
        on:click={() => runSync(repo.id, repo.name, 'pull')}
      >
        <svg viewBox="0 0 16 16" class="w-4 h-4" fill="none" stroke="currentColor"
             stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M8 2v7.5"/>
          <path d="M4.5 6 8 9.5 11.5 6"/>
          <path d="M3 13h10"/>
        </svg>
      </button>
      <!-- Push -->
      <button
        class="p-1 rounded transition-colors disabled:cursor-default
               {busy === 'push'
                 ? 'text-success animate-pulse'
                 : busy
                   ? 'text-success opacity-40'
                   : 'text-success hover:bg-border-muted'}"
        disabled={!!busy}
        title={busy === 'push' ? 'Pushing… (in progress)' : 'Push'}
        aria-label="Push"
        on:click={() => runSync(repo.id, repo.name, 'push')}
      >
        <svg viewBox="0 0 16 16" class="w-4 h-4" fill="none" stroke="currentColor"
             stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M8 14V6.5"/>
          <path d="M4.5 10 8 6.5 11.5 10"/>
          <path d="M3 3h10"/>
        </svg>
      </button>
      <span class="text-xs text-fg-muted ml-1">branch</span>
      <BranchMenu
        {repo}
        {branches}
        current={currentBranch}
        {diverged}
        dirty={!!(st && st.changed_files > 0)}
        divergedTitle={diverged
          ? `HEAD diverged — ${st.behind} commit${st.behind === 1 ? '' : 's'} to pull, ${st.ahead} to push`
          : ''}
        on:changed={onBranchChanged}
      />
      {#if st && (st.ahead > 0 || st.behind > 0)}
        <span class="flex items-center gap-1.5 text-xs font-semibold
                     {diverged ? 'text-attention' : 'text-accent'}">
          {#if st.behind > 0}<span title="commits to pull">↓{st.behind}</span>{/if}
          {#if st.ahead > 0}<span title="commits to push">↑{st.ahead}</span>{/if}
        </span>
      {/if}
    </div>
  </div>

  <div class="flex-1 overflow-hidden">
    {#key dataVersion}
      {#if $activeTab === 'changes'}
        <Changes {repo} on:changed={onBranchChanged} />
      {:else if $activeTab === 'history'}
        <History
          {repo}
          localBranch={st?.local_branch}
          branch={currentBranch}
          on:changed={onBranchChanged}
        />
      {:else}
        <RepoSettings {repo} />
      {/if}
    {/key}
  </div>
</div>
