<script>
  import { fly } from 'svelte/transition';
  import { toasts, dismissToast } from '../lib/toasts.js';

  function runAction(t) {
    dismissToast(t.id);
    t.action?.run?.();
  }

  function accent(kind) {
    if (kind === 'success') return 'border-l-success';
    if (kind === 'error') return 'border-l-danger';
    return 'border-l-accent';
  }
</script>

<div
  class="fixed bottom-4 right-4 z-[60] flex flex-col gap-2 w-80 pointer-events-none"
>
  {#each $toasts as t (t.id)}
    <div
      class="pointer-events-auto rounded-md border border-border border-l-4 {accent(t.kind)}
             bg-canvas-subtle shadow-2xl px-3 py-2"
      transition:fly={{ x: 24, duration: 160 }}
    >
      <div class="flex items-start gap-2">
        <p class="text-sm text-fg flex-1">{t.message}</p>
        <button
          class="text-fg-muted hover:text-fg text-base leading-none shrink-0"
          aria-label="Dismiss"
          on:click={() => dismissToast(t.id)}
        >×</button>
      </div>
      {#if t.action}
        <div class="mt-2 flex justify-end">
          <button class="btn-mini" on:click={() => runAction(t)}>{t.action.label}</button>
        </div>
      {/if}
    </div>
  {/each}
</div>
