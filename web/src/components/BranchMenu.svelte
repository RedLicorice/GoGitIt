<script>
  import { createEventDispatcher } from 'svelte';
  import { api } from '../lib/api.js';
  import { addToast } from '../lib/toasts.js';
  import ConfirmDialog from './ConfirmDialog.svelte';

  export let repo;
  export let branches = [];
  export let current = '';
  export let diverged = false;
  export let divergedTitle = '';
  export let dirty = false; // worktree has uncommitted changes

  const dispatch = createEventDispatcher();

  let open = false;
  let working = false;
  let newName = '';
  let confirmState = null; // { kind: 'delete' | 'merge', name }
  let switchState = null; // target branch awaiting a dirty-switch choice

  $: locals = branches.filter((b) => !b.is_remote);

  async function op(fn, successMsg) {
    working = true;
    try {
      await fn();
      if (successMsg) addToast({ kind: 'success', message: successMsg });
      dispatch('changed');
      open = false;
      newName = '';
    } catch (e) {
      addToast({ kind: 'error', message: e.message, timeout: 9000 });
    } finally {
      working = false;
    }
  }

  function switchTo(name) {
    if (name === current) {
      open = false;
      return;
    }
    if (dirty) {
      switchState = name; // ask: bring changes along / stash / cancel
      return;
    }
    doSwitch(name, false);
  }
  function doSwitch(name, stash) {
    switchState = null;
    op(
      () => api.switchBranch(repo.id, name, stash),
      stash ? `Stashed changes — switched to ${name}` : `Switched to ${name}`,
    );
  }
  function create() {
    const n = newName.trim();
    if (n) op(() => api.createBranch(repo.id, n), `Created branch ${n}`);
  }
  // Merge and delete both go through a confirmation dialog — easy to misclick.
  function confirmMessage(s) {
    if (!s) return '';
    if (s.kind === 'merge') {
      return `Merge "${s.name}" into "${current}"?\n` +
        'Fast-forwards the current branch to that branch.';
    }
    return `Delete branch "${s.name}"?\n` +
      'Commits reachable only from this branch may become unreachable.';
  }
  function resolveConfirm() {
    const s = confirmState;
    confirmState = null;
    if (!s) return;
    if (s.kind === 'merge') {
      op(() => api.mergeBranch(repo.id, s.name), `Merged ${s.name} into ${current}`);
    } else {
      op(() => api.deleteBranch(repo.id, s.name), `Deleted branch ${s.name}`);
    }
  }
</script>

<div class="relative">
  <button
    class="flex items-center gap-1 text-sm font-mono text-fg bg-canvas-inset
           border border-border px-2 py-0.5 rounded hover:border-accent transition-colors
           {diverged ? 'border-l-4 border-l-attention' : ''}"
    title={diverged ? divergedTitle : 'Branches'}
    on:click={() => (open = !open)}
  >
    <span class="truncate max-w-[10rem]">{current || '—'}</span>
    <span class="text-fg-muted text-[10px]">▾</span>
  </button>

  {#if open}
    <button
      class="fixed inset-0 z-40 cursor-default"
      aria-label="Close branch menu"
      on:click={() => (open = false)}
    ></button>
    <div
      class="absolute right-0 mt-1 w-64 z-50 rounded-md border border-border
             bg-canvas-subtle shadow-2xl overflow-hidden"
    >
      <div class="px-3 py-1.5 border-b border-border text-xs uppercase tracking-wider
                  text-fg-muted font-semibold">
        Branches
      </div>
      <ul class="max-h-64 overflow-y-auto py-1">
        {#each locals as b (b.name)}
          <li class="group flex items-center gap-1 pr-2 hover:bg-border-muted">
            <button
              class="flex-1 min-w-0 text-left px-3 py-1.5 text-sm truncate
                     {b.name === current ? 'text-fg font-semibold' : 'text-fg-muted'}"
              disabled={working}
              on:click={() => switchTo(b.name)}
            >
              {b.name === current ? '● ' : ''}{b.name}
            </button>
            {#if b.name !== current}
              <div class="flex gap-1 shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
                <button class="btn-mini" disabled={working}
                        on:click={() => (confirmState = { kind: 'merge', name: b.name })}>merge</button>
                <button class="btn-mini hover:!text-danger" disabled={working}
                        on:click={() => (confirmState = { kind: 'delete', name: b.name })}>del</button>
              </div>
            {/if}
          </li>
        {/each}
        {#if locals.length === 0}
          <li class="px-3 py-2 text-sm text-fg-muted">No branches.</li>
        {/if}
      </ul>
      <div class="border-t border-border p-2 flex gap-2">
        <input
          class="input"
          placeholder="new branch name"
          bind:value={newName}
          disabled={working}
          on:keydown={(e) => e.key === 'Enter' && create()}
        />
        <button
          class="btn btn-primary shrink-0 justify-center"
          disabled={working || !newName.trim()}
          on:click={create}
        >Create</button>
      </div>
    </div>
  {/if}
</div>

<ConfirmDialog
  open={!!confirmState}
  title={confirmState?.kind === 'merge' ? 'Merge branch' : 'Delete branch'}
  message={confirmMessage(confirmState)}
  confirmLabel={confirmState?.kind === 'merge' ? 'Merge' : 'Delete'}
  danger={confirmState?.kind === 'delete'}
  on:confirm={resolveConfirm}
  on:cancel={() => (confirmState = null)}
/>

{#if switchState}
  <div class="fixed inset-0 z-50 flex items-center justify-center">
    <button
      class="absolute inset-0 bg-black/60 cursor-default"
      aria-label="Cancel"
      on:click={() => (switchState = null)}
    ></button>
    <div
      class="relative w-full max-w-sm mx-4 rounded-lg border border-border
             bg-canvas-subtle shadow-2xl"
      role="dialog"
      aria-modal="true"
    >
      <div class="px-4 py-3 border-b border-border">
        <h3 class="text-sm font-semibold text-fg">Switch to {switchState}</h3>
      </div>
      <p class="p-4 text-xs text-fg-muted">
        You have uncommitted changes. Bring them along to
        <span class="font-mono text-fg">{switchState}</span>, or stash them aside
        first (pop them later from the Changes tab)?
      </p>
      <div class="px-4 py-3 border-t border-border flex flex-col gap-2">
        <button
          class="btn btn-primary justify-center"
          disabled={working}
          on:click={() => doSwitch(switchState, false)}
        >Bring changes along</button>
        <button
          class="btn justify-center"
          disabled={working}
          on:click={() => doSwitch(switchState, true)}
        >Stash changes &amp; switch clean</button>
        <button
          class="btn justify-center"
          disabled={working}
          on:click={() => (switchState = null)}
        >Cancel</button>
      </div>
    </div>
  </div>
{/if}
