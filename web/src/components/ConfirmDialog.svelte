<script>
  import { createEventDispatcher } from 'svelte';

  export let open = false;
  export let title = 'Confirm';
  export let message = '';
  export let confirmLabel = 'Confirm';
  export let cancelLabel = 'Cancel';
  export let danger = false;

  const dispatch = createEventDispatcher();

  function confirm() {
    dispatch('confirm');
  }
  function cancel() {
    dispatch('cancel');
  }
  function onKey(e) {
    if (!open) return;
    if (e.key === 'Escape') cancel();
    else if (e.key === 'Enter') confirm();
  }
</script>

<svelte:window on:keydown={onKey} />

{#if open}
  <div class="fixed inset-0 z-50 flex items-center justify-center">
    <!-- Backdrop (a button so keyboard/dismiss is accessible) -->
    <button
      class="absolute inset-0 bg-black/60 cursor-default"
      aria-label="Dismiss dialog"
      on:click={cancel}
    ></button>

    <div
      class="relative w-full max-w-md mx-4 rounded-lg border border-border
             bg-canvas-subtle shadow-2xl"
      role="dialog"
      aria-modal="true"
      aria-label={title}
    >
      <div class="px-4 py-3 border-b border-border">
        <h3 class="text-sm font-semibold text-fg">{title}</h3>
      </div>
      <div class="px-4 py-4 text-sm text-fg-muted whitespace-pre-line">{message}</div>
      <div class="px-4 py-3 border-t border-border flex justify-end gap-2">
        <button class="btn" on:click={cancel}>{cancelLabel}</button>
        <button
          class="btn {danger ? 'btn-danger' : 'btn-primary'}"
          on:click={confirm}
        >{confirmLabel}</button>
      </div>
    </div>
  </div>
{/if}
