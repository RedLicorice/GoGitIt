<script>
  import { api } from '../lib/api.js';
  import { isMobile } from '../lib/stores.js';
  import { runSync } from '../lib/sync.js';
  import DiffViewer from './DiffViewer.svelte';
  import DiffModeToggle from './DiffModeToggle.svelte';
  import ShaPill from './ShaPill.svelte';
  import TreeView from './TreeView.svelte';

  export let repo;
  export let localBranch = false; // current branch has no remote tracking ref
  export let branch = '';

  let viewMode = 'history'; // 'history' (3-column) | 'tree' (commit graph)

  let commits = [];
  let loading = true;
  let error = null;
  let selected = null;

  // Mobile drill-down: which of the 3 columns is showing. Desktop ignores it.
  let mobilePane = 'commits'; // 'commits' | 'files' | 'diff'

  // Affected files for the selected commit.
  let commitFiles = [];
  let filesLoading = false;
  let filesError = null;
  let loadedHash = null;

  // Files picked in the Commit Files column. Empty = show the whole commit.
  let selectedPaths = new Set();
  // Accordion expansion state in the diff panel, keyed by path.
  let expanded = {};

  async function load() {
    loading = true;
    error = null;
    loadedHash = null;
    mobilePane = 'commits';
    try {
      commits = await api.log(repo.id);
      selected = commits[0] || null;
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function loadFiles(hash) {
    loadedHash = hash;
    filesLoading = true;
    filesError = null;
    commitFiles = [];
    selectedPaths = new Set();
    try {
      const files = await api.commitDiff(repo.id, hash);
      commitFiles = files || [];
    } catch (e) {
      filesError = e.message;
    } finally {
      filesLoading = false;
    }
  }

  // Pick files in the Commit Files column. Plain click selects just that file
  // (or clears it when it was the sole pick); Shift+click toggles membership
  // for multi-select.
  function pickFile(path, additive) {
    if (additive) {
      const next = new Set(selectedPaths);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      selectedPaths = next;
    } else if (selectedPaths.size === 1 && selectedPaths.has(path)) {
      selectedPaths = new Set();
    } else {
      selectedPaths = new Set([path]);
    }
  }

  // Mobile drill-down navigation.
  function pickCommit(c) {
    selected = c;
    if ($isMobile) mobilePane = 'files';
  }
  function pickFileMobile(path, additive) {
    pickFile(path, additive);
    if ($isMobile) mobilePane = 'diff';
  }
  function mobileBack() {
    mobilePane = mobilePane === 'diff' ? 'files' : 'commits';
  }

  // Toggle one accordion section in the diff panel.
  function toggleExpand(path) {
    expanded = { ...expanded, [path]: !expanded[path] };
  }

  // Files shown in the diff panel: the selection, or all files when empty.
  $: displayFiles =
    selectedPaths.size === 0
      ? commitFiles
      : commitFiles.filter((f) => selectedPaths.has(f.path));

  // Show the commit-message heading only in the "whole commit" view.
  $: showCommitHeader = selectedPaths.size === 0;

  // Reset expansion whenever the displayed set changes: auto-expand a lone
  // file, otherwise expand only the first (and only in the whole-commit view).
  $: applyDefaultExpansion(displayFiles, selectedPaths);
  function applyDefaultExpansion(files, sel) {
    const next = {};
    if (files.length === 1) {
      next[files[0].path] = true;
    } else if (sel.size === 0 && files.length) {
      next[files[0].path] = true;
    }
    expanded = next;
  }

  $: if (repo) load();
  $: if (selected && selected.hash !== loadedHash) loadFiles(selected.hash);

  function formatDate(d) {
    return new Date(d).toLocaleString();
  }
  function formatShort(d) {
    return new Date(d).toLocaleString(undefined, {
      year: '2-digit',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  }
  function firstLine(msg) {
    return (msg || '').split('\n')[0];
  }
  function statusColor(s) {
    if (s === 'A') return 'text-success';
    if (s === 'D') return 'text-danger';
    if (s === 'R') return 'text-attention';
    return 'text-accent';
  }
</script>

<div class="h-full flex flex-col">
  <div class="shrink-0 flex items-center border-b border-border bg-canvas-subtle px-2">
    <button
      class="px-3 py-1.5 text-xs font-medium border-b-2 transition-colors
             {viewMode === 'history'
               ? 'border-accent text-fg'
               : 'border-transparent text-fg-muted hover:text-fg'}"
      on:click={() => (viewMode = 'history')}
    >History</button>
    <button
      class="px-3 py-1.5 text-xs font-medium border-b-2 transition-colors
             {viewMode === 'tree'
               ? 'border-accent text-fg'
               : 'border-transparent text-fg-muted hover:text-fg'}"
      on:click={() => (viewMode = 'tree')}
    >Tree</button>
  </div>

  {#if localBranch && viewMode === 'history'}
    <div
      class="shrink-0 flex items-center gap-3 px-4 py-2 text-sm
             bg-attention/10 border-b border-attention/30 text-attention"
    >
      <span class="flex-1">
        Branch <span class="font-semibold">{branch || 'this'}</span> is local —
        not on any remote yet. Its commits read as “unpushed” until the branch
        is pushed.
      </span>
      <button
        class="btn-mini shrink-0"
        on:click={() => runSync(repo.id, repo.name, 'push')}
      >Push branch</button>
    </div>
  {/if}

  {#if viewMode === 'tree'}
    <div class="flex-1 min-h-0">
      <TreeView {repo} on:changed />
    </div>
  {:else}
  {#if $isMobile && mobilePane !== 'commits'}
    <button
      class="md:hidden shrink-0 flex items-center gap-1.5 px-3 py-2 text-sm
             text-accent border-b border-border bg-canvas-subtle"
      on:click={mobileBack}
    >
      <span class="text-base leading-none">‹</span>
      {mobilePane === 'diff' ? 'Commit files' : 'Commits'}
    </button>
  {/if}
  <div class="flex-1 flex min-h-0">
  <!-- Column 1: commit list -->
  <div
    class="w-full md:w-80 md:shrink-0 min-w-0 border-r border-border flex-col overflow-hidden
           {mobilePane === 'commits' ? 'flex' : 'hidden'} md:flex"
  >
    <div class="px-3 py-2 border-b border-border flex items-center justify-between">
      <span class="text-xs uppercase tracking-wider text-fg-muted font-semibold">Commits</span>
      <button class="text-xs text-fg-muted hover:text-fg" on:click={load}>Refresh</button>
    </div>

    <div class="flex-1 overflow-y-auto">
      {#if loading}
        <p class="px-3 py-4 text-sm text-fg-muted">Loading…</p>
      {:else if error}
        <p class="px-3 py-4 text-sm text-danger">{error}</p>
      {:else if commits.length === 0}
        <p class="px-3 py-4 text-sm text-fg-muted">No commits.</p>
      {:else}
        <ul>
          {#each commits as c (c.hash)}
            <li
              class="border-b border-border-muted hover:bg-border-muted transition-colors
                     {selected?.hash === c.hash ? 'bg-accent-subtle' : ''}"
            >
              <div class="px-3 py-2">
                <button
                  class="w-full text-left flex items-center gap-2"
                  on:click={() => pickCommit(c)}
                >
                  <span class="text-sm text-fg truncate flex-1">{firstLine(c.message)}</span>
                  {#if !c.pushed}
                    <span
                      class="shrink-0 text-[10px] leading-none px-1 py-0.5 rounded
                             bg-attention/15 text-attention"
                      title="Not pushed to remote"
                    >unpushed</span>
                  {/if}
                </button>
                <div class="text-xs text-fg-muted mt-1 flex items-center gap-1.5">
                  <ShaPill sha={c.hash} label={c.short_hash} />
                  <button
                    class="flex items-center gap-1.5 min-w-0 text-left"
                    on:click={() => pickCommit(c)}
                  >
                    <span class="shrink-0">·</span>
                    <span class="truncate">{c.author}</span>
                    <span class="shrink-0">·</span>
                    <span class="shrink-0">{formatShort(c.date)}</span>
                  </button>
                </div>
              </div>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>

  <!-- Column 2: commit files (only once a commit is selected) -->
  {#if selected}
    <div
      class="w-full md:w-64 md:shrink-0 min-w-0 border-r border-border flex-col overflow-hidden
             {mobilePane === 'files' ? 'flex' : 'hidden'} md:flex"
    >
      <div class="px-3 py-2 border-b border-border flex items-center justify-between">
        <span class="text-xs uppercase tracking-wider text-fg-muted font-semibold">
          Commit Files
        </span>
        {#if selectedPaths.size > 0}
          <button
            class="text-xs text-fg-muted hover:text-fg"
            on:click={() => (selectedPaths = new Set())}
          >Clear</button>
        {/if}
      </div>

      <div class="flex-1 overflow-y-auto">
        {#if filesLoading}
          <p class="px-3 py-4 text-sm text-fg-muted">Loading…</p>
        {:else if filesError}
          <p class="px-3 py-4 text-sm text-danger">{filesError}</p>
        {:else if commitFiles.length === 0}
          <p class="px-3 py-4 text-sm text-fg-muted">No file changes.</p>
        {:else}
          <ul>
            {#each commitFiles as fd (fd.path)}
              <li>
                <button
                  class="w-full text-left px-3 py-1.5 flex items-center gap-2 select-none
                         hover:bg-border-muted transition-colors
                         {selectedPaths.has(fd.path) ? 'bg-accent-subtle' : ''}"
                  title={fd.status === 'R' && fd.old_path ? `${fd.old_path} → ${fd.path}` : fd.path}
                  on:click={(e) => pickFileMobile(fd.path, e.shiftKey)}
                >
                  <span class="font-mono text-xs w-4 shrink-0 {statusColor(fd.status)}">{fd.status}</span>
                  <span class="text-sm text-fg truncate flex-1">{fd.path}</span>
                  {#if fd.additions}<span class="text-xs text-success shrink-0">+{fd.additions}</span>{/if}
                  {#if fd.deletions}<span class="text-xs text-danger shrink-0">−{fd.deletions}</span>{/if}
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    </div>
  {/if}

  <!-- Column 3: diff panel -->
  <div
    class="flex-1 min-w-0 flex-col overflow-hidden
           {mobilePane === 'diff' ? 'flex' : 'hidden'} md:flex"
  >
    {#if !selected}
      <div class="flex-1 flex items-center justify-center text-fg-muted text-sm">
        No commit selected.
      </div>
    {:else}
      <div class="px-4 py-2 border-b border-border flex items-center justify-between shrink-0">
        <span class="text-sm text-fg-muted">
          {#if selectedPaths.size === 0}
            Commit detail
          {:else}
            {selectedPaths.size} file{selectedPaths.size === 1 ? '' : 's'} selected
          {/if}
        </span>
        {#if commitFiles.length}
          <DiffModeToggle />
        {/if}
      </div>

      <div class="flex-1 overflow-y-auto p-3 md:p-6">
        <div class="space-y-4">
          {#if showCommitHeader}
            <h2 class="text-xl font-semibold text-fg">{firstLine(selected.message)}</h2>
            {#if selected.message.split('\n').length > 1}
              <pre class="text-sm text-fg-muted whitespace-pre-wrap font-sans">{selected.message
                  .split('\n').slice(1).join('\n').trim()}</pre>
            {/if}
            <div class="flex items-center gap-4 text-sm text-fg-muted">
              <span><span class="text-fg">{selected.author}</span> &lt;{selected.email}&gt;</span>
              <span>·</span>
              <span>{formatDate(selected.date)}</span>
            </div>
            <div class="space-y-1.5 text-xs text-fg-muted">
              <div class="flex items-center gap-2">
                <span class="font-mono">commit</span>
                <ShaPill sha={selected.hash} />
              </div>
              {#if selected.parents?.length}
                <div class="flex items-center gap-2 flex-wrap">
                  <span class="font-mono">parent</span>
                  {#each selected.parents as p}
                    <ShaPill sha={p} />
                  {/each}
                </div>
              {/if}
            </div>
          {/if}

          <div class:pt-4={showCommitHeader} class:border-t={showCommitHeader} class="border-border">
            {#if showCommitHeader}
              <div class="text-sm font-semibold text-fg mb-2">
                {#if filesLoading}
                  Loading files…
                {:else}
                  {commitFiles.length} changed file{commitFiles.length === 1 ? '' : 's'}
                {/if}
              </div>
            {/if}

            {#if filesError}
              <p class="text-sm text-danger">{filesError}</p>
            {:else if filesLoading && !showCommitHeader}
              <p class="text-sm text-fg-muted">Loading files…</p>
            {:else if !filesLoading && displayFiles.length === 0}
              <p class="text-sm text-fg-muted italic">No file changes.</p>
            {:else}
              <ul class="-mx-3 md:-mx-6 border-y border-border divide-y divide-border">
                {#each displayFiles as fd (fd.path)}
                  <li class="overflow-hidden">
                    <button
                      class="w-full flex items-center gap-2 px-3 md:px-6 py-2 text-left
                             bg-canvas-subtle hover:bg-border-muted transition-colors"
                      on:click={() => toggleExpand(fd.path)}
                    >
                      <span class="font-mono text-xs w-4 {statusColor(fd.status)}">{fd.status}</span>
                      <span class="text-sm text-fg truncate flex-1">
                        {#if fd.status === 'R' && fd.old_path}{fd.old_path} → {/if}{fd.path}
                      </span>
                      {#if fd.additions}<span class="text-xs text-success">+{fd.additions}</span>{/if}
                      {#if fd.deletions}<span class="text-xs text-danger">−{fd.deletions}</span>{/if}
                      <span class="text-fg-muted text-xs w-3">{expanded[fd.path] ? '▾' : '▸'}</span>
                    </button>
                    {#if expanded[fd.path]}
                      <div class="border-t border-border">
                        <DiffViewer file={fd} />
                      </div>
                    {/if}
                  </li>
                {/each}
              </ul>
            {/if}
          </div>
        </div>
      </div>
    {/if}
  </div>
  </div>
  {/if}
</div>
