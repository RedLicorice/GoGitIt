<script>
  import { api } from '../lib/api.js';
  import { addToast } from '../lib/toasts.js';
  import { loadRepos, selectedRepoId } from '../lib/stores.js';

  export let repo;

  let open = false;
  let loading = false;
  let working = false;
  let loaded = false;
  let entries = [];
  let error = null;

  async function load() {
    loading = true;
    error = null;
    try {
      entries = (await api.submodules(repo.id)) || [];
      loaded = true;
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  function toggle() {
    open = !open;
    if (open && !loaded) load();
  }

  async function updateAll(remote) {
    working = true;
    try {
      await api.submodulesUpdate(repo.id, remote);
      addToast({
        kind: 'success',
        message: remote
          ? 'Submodules pulled to latest tracked commits'
          : 'Submodules initialized & updated',
      });
      await load();
    } catch (e) {
      addToast({ kind: 'error', message: e.message, timeout: 9000 });
    } finally {
      working = false;
    }
  }

  async function addToGogitIt(sub) {
    working = true;
    try {
      const name = sub.path.split('/').filter(Boolean).pop() || sub.path;
      await api.addRepo(name, sub.abs_path);
      addToast({ kind: 'success', message: `Added ${sub.path} to GoGitIt` });
      await loadRepos();
      await load();
    } catch (e) {
      addToast({ kind: 'error', message: e.message, timeout: 9000 });
    } finally {
      working = false;
    }
  }

  function openSubmodule(sub) {
    if (sub.registered_id) {
      selectedRepoId.set(sub.registered_id);
      open = false;
    }
  }

  // State badge per entry.
  function stateLabel(s) {
    if (!s.initialized) return { text: 'not initialized', class: 'text-fg-muted' };
    if (s.has_conflicts) return { text: 'conflicts', class: 'text-danger' };
    if (s.out_of_sync) return { text: 'out of sync', class: 'text-attention' };
    return { text: 'in sync', class: 'text-success' };
  }
</script>

<div class="relative">
  <button
    class="inline-flex items-center gap-1 text-xs text-accent px-1.5 py-0.5 rounded
           border border-border bg-canvas-inset hover:border-accent transition-colors"
    title="Submodules"
    on:click={toggle}
  >
    <svg viewBox="0 0 16 16" class="w-3.5 h-3.5" fill="currentColor" aria-hidden="true">
      <path d="M5 5.372v.878c0 .414.336.75.75.75h4.5a.75.75 0 0 0 .75-.75v-.878a2.25 2.25 0 1 1 1.5 0v.878a2.25 2.25 0 0 1-2.25 2.25h-1.5v2.128a2.251 2.251 0 1 1-1.5 0V8.5h-1.5A2.25 2.25 0 0 1 3.5 6.25v-.878a2.25 2.25 0 1 1 1.5 0ZM5 3.25a.75.75 0 1 0-1.5 0 .75.75 0 0 0 1.5 0Zm6.75.75a.75.75 0 1 0 0-1.5.75.75 0 0 0 0 1.5Zm-3 8.75a.75.75 0 1 0-1.5 0 .75.75 0 0 0 1.5 0Z"/>
    </svg>
    Submodules
    <span class="text-fg-muted text-[10px]">▾</span>
  </button>

  {#if open}
    <button
      class="fixed inset-0 z-40 cursor-default"
      aria-label="Close submodule menu"
      on:click={() => (open = false)}
    ></button>
    <div
      class="absolute right-0 mt-1 w-80 z-50 rounded-md border border-border
             bg-canvas-subtle shadow-2xl overflow-hidden"
    >
      <div class="px-3 py-1.5 border-b border-border text-xs uppercase tracking-wider
                  text-fg-muted font-semibold flex items-center justify-between">
        <span>Submodules</span>
        {#if entries.length}
          <span class="normal-case">{entries.length}</span>
        {/if}
      </div>

      {#if loading}
        <p class="px-3 py-3 text-sm text-fg-muted">Loading…</p>
      {:else if error}
        <p class="px-3 py-3 text-sm text-danger">{error}</p>
      {:else if entries.length === 0}
        <p class="px-3 py-3 text-sm text-fg-muted">No submodules in this repo.</p>
      {:else}
        <ul class="max-h-72 overflow-y-auto divide-y divide-border">
          {#each entries as s (s.path)}
            {@const st = stateLabel(s)}
            <li class="px-3 py-2 space-y-1">
              <div class="flex items-center gap-2 min-w-0">
                <span class="text-sm text-fg truncate flex-1" title={s.path}>{s.path}</span>
                <span class="text-[10px] uppercase tracking-wider shrink-0 {st.class}">{st.text}</span>
              </div>
              <div class="flex flex-wrap items-center gap-1.5">
                {#if s.registered_id}
                  <button class="btn-mini" disabled={working} on:click={() => openSubmodule(s)}>
                    Open {s.registered_name}
                  </button>
                {:else if s.initialized}
                  <button class="btn-mini" disabled={working} on:click={() => addToGogitIt(s)}>
                    Add to GoGitIt
                  </button>
                {:else}
                  <span class="text-[11px] text-fg-muted italic">
                    run “Update” below to check it out
                  </span>
                {/if}
              </div>
            </li>
          {/each}
        </ul>
      {/if}

      <div class="border-t border-border p-2 flex flex-col gap-1.5">
        <button
          class="btn-mini justify-center"
          disabled={working}
          title="git submodule update --init --recursive"
          on:click={() => updateAll(false)}
        >Update (--init --recursive)</button>
        <button
          class="btn-mini justify-center"
          disabled={working}
          title="git submodule update --init --recursive --remote"
          on:click={() => updateAll(true)}
        >Update --remote (pull latest)</button>
      </div>
    </div>
  {/if}
</div>
