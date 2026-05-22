<script>
  import { api } from '../lib/api.js';
  import ConfirmDialog from './ConfirmDialog.svelte';

  export let repo;

  let remotes = [];
  let loading = true;
  let error = null;
  let working = false;

  // Editable URL per remote, keyed by remote name.
  let urlEdits = {};

  let newName = '';
  let newUrl = '';

  let confirmState = null; // { name, message }

  let lastRepoId = null;
  $: if (repo && repo.id !== lastRepoId) {
    lastRepoId = repo.id;
    load();
  }

  function resetEdits() {
    const m = {};
    for (const r of remotes) m[r.name] = (r.urls && r.urls[0]) || '';
    urlEdits = m;
  }

  async function load() {
    loading = true;
    error = null;
    try {
      remotes = await api.remotes(repo.id);
      resetEdits();
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function saveUrl(name) {
    working = true;
    error = null;
    try {
      remotes = await api.setRemoteUrl(repo.id, name, (urlEdits[name] || '').trim());
      resetEdits();
    } catch (e) {
      error = e.message;
    } finally {
      working = false;
    }
  }

  async function addRemote() {
    if (!newName.trim() || !newUrl.trim()) return;
    working = true;
    error = null;
    try {
      remotes = await api.addRemote(repo.id, newName.trim(), newUrl.trim());
      newName = '';
      newUrl = '';
      resetEdits();
    } catch (e) {
      error = e.message;
    } finally {
      working = false;
    }
  }

  function confirmRemove(name) {
    confirmState = {
      name,
      message:
        `Remove remote "${name}"?\n` +
        'This edits local git config only — nothing on the remote is touched.',
    };
  }
  async function doRemove() {
    const name = confirmState?.name;
    confirmState = null;
    if (!name) return;
    working = true;
    error = null;
    try {
      remotes = await api.removeRemote(repo.id, name);
      resetEdits();
    } catch (e) {
      error = e.message;
    } finally {
      working = false;
    }
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="max-w-2xl mx-auto p-6 space-y-6">
    <div>
      <h2 class="text-base font-semibold text-fg">Remotes</h2>
      <p class="text-xs text-fg-muted mt-0.5">
        Remotes this repository fetches from and pushes to. Changes here edit
        local git config only.
      </p>
    </div>

    {#if error}
      <p class="text-sm text-danger bg-danger/10 rounded px-3 py-2">{error}</p>
    {/if}

    {#if loading}
      <p class="text-sm text-fg-muted">Loading…</p>
    {:else}
      {#if remotes.length === 0}
        <p class="text-sm text-fg-muted italic">No remotes configured.</p>
      {:else}
        <ul class="space-y-2">
          {#each remotes as r (r.name)}
            {@const orig = (r.urls && r.urls[0]) || ''}
            <li class="border border-border rounded-md p-3 space-y-2">
              <div class="flex items-center justify-between">
                <span class="font-mono text-sm text-fg">{r.name}</span>
                <button
                  class="btn-mini hover:!text-danger"
                  disabled={working}
                  on:click={() => confirmRemove(r.name)}
                >Remove</button>
              </div>
              <div class="flex gap-2">
                <input
                  class="input"
                  bind:value={urlEdits[r.name]}
                  disabled={working}
                  placeholder="https://… or git@…"
                />
                <button
                  class="btn btn-primary shrink-0 justify-center"
                  disabled={working ||
                    !(urlEdits[r.name] || '').trim() ||
                    (urlEdits[r.name] || '').trim() === orig}
                  on:click={() => saveUrl(r.name)}
                >Save</button>
              </div>
            </li>
          {/each}
        </ul>
      {/if}

      <div class="border-t border-border pt-4 space-y-2">
        <h3 class="text-sm font-semibold text-fg">Add a remote</h3>
        <div class="flex gap-2">
          <input
            class="input w-36 shrink-0"
            placeholder="name (origin)"
            bind:value={newName}
            disabled={working}
          />
          <input
            class="input"
            placeholder="https://… or git@…"
            bind:value={newUrl}
            disabled={working}
          />
          <button
            class="btn btn-primary shrink-0 justify-center"
            disabled={working || !newName.trim() || !newUrl.trim()}
            on:click={addRemote}
          >Add</button>
        </div>
      </div>
    {/if}
  </div>
</div>

<ConfirmDialog
  open={!!confirmState}
  title="Remove remote"
  message={confirmState?.message || ''}
  confirmLabel="Remove"
  danger
  on:confirm={doRemove}
  on:cancel={() => (confirmState = null)}
/>
