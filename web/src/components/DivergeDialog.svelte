<script>
  import { divergenceDialog } from '../lib/stores.js';
  import { runRebasePush } from '../lib/sync.js';

  $: info = $divergenceDialog;

  function cancel() {
    divergenceDialog.set(null);
  }

  function rebaseAndPush() {
    if (!info) return;
    runRebasePush(info.repoId, info.repoName);
  }

  function onKey(e) {
    if (!info) return;
    if (e.key === 'Escape') cancel();
  }
</script>

<svelte:window on:keydown={onKey} />

{#if info}
  <!-- Backdrop -->
  <button
    class="fixed inset-0 z-40 bg-black/60 cursor-default"
    aria-label="Dismiss dialog"
    on:click={cancel}
  />

  <!-- Dialog -->
  <div
    class="fixed z-50 left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2
           w-full max-w-lg mx-4 bg-canvas-subtle border border-border rounded-lg
           shadow-2xl flex flex-col overflow-hidden"
    role="dialog"
    aria-modal="true"
    aria-labelledby="diverge-title"
  >
    <div class="px-4 py-3 border-b border-border">
      <h3 id="diverge-title" class="text-sm font-semibold text-fg">
        Branch has diverged from remote
      </h3>
    </div>

    <div class="px-4 py-4 flex-1 overflow-y-auto space-y-3">
      <p class="text-sm text-fg-muted">
        To pull, your local commits will be rebased on top of the remote and the
        result pushed back. If the rebase produces conflicts the operation stops
        and you resolve them manually.
      </p>

      {#if info.dirty}
        <p class="text-sm text-attention border border-attention/30 bg-attention/10 rounded px-3 py-2">
          Your working tree has uncommitted changes — they will be stashed before
          the rebase and restored afterwards.
        </p>
      {/if}

      <div class="grid grid-cols-2 gap-3">
        <div>
          <p class="text-xs font-medium text-fg mb-1">
            Local-only commits
            <span class="text-fg-muted font-normal">(↑{info.ahead})</span>
          </p>
          <ul class="font-mono text-xs text-fg-muted space-y-0.5 max-h-28 overflow-y-auto">
            {#if info.local_commits && info.local_commits.length}
              {#each info.local_commits as c}
                <li class="truncate">{c}</li>
              {/each}
            {:else}
              <li class="italic">none</li>
            {/if}
          </ul>
        </div>
        <div>
          <p class="text-xs font-medium text-fg mb-1">
            Remote-only commits
            <span class="text-fg-muted font-normal">(↓{info.behind})</span>
          </p>
          <ul class="font-mono text-xs text-fg-muted space-y-0.5 max-h-28 overflow-y-auto">
            {#if info.remote_commits && info.remote_commits.length}
              {#each info.remote_commits as c}
                <li class="truncate">{c}</li>
              {/each}
            {:else}
              <li class="italic">none</li>
            {/if}
          </ul>
        </div>
      </div>
    </div>

    <div class="px-4 py-3 border-t border-border flex justify-end gap-2">
      <button class="btn" on:click={cancel}>Cancel</button>
      <button
        class="btn border-attention-emphasis bg-attention-emphasis text-white
               hover:bg-attention hover:border-attention"
        on:click={rebaseAndPush}
      >
        Rebase &amp; Push
      </button>
    </div>
  </div>
{/if}
