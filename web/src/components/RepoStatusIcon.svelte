<script>
  // One status indicator for a repo row. `state` is the cascade result:
  //   { kind: 'op',      op: 'fetch'|'pull'|'push' }
  //   { kind: 'error' } | { kind: 'merge' } | { kind: 'clean' }
  //   { kind: 'changes'|'ahead'|'behind', n: number }
  // A null state renders nothing.
  export let state;

  const OP_GLYPH = { fetch: '↻', pull: '↓', push: '↑' };
  const OP_COLOR = { fetch: 'text-attention', pull: 'text-success', push: 'text-success' };
  const OP_LABEL = { fetch: 'Fetching…', pull: 'Pulling…', push: 'Pushing…' };

  $: label = makeLabel(state);
  function makeLabel(s) {
    if (!s) return '';
    switch (s.kind) {
      case 'op': return OP_LABEL[s.op] || 'Working…';
      case 'error': return 'Last operation failed';
      case 'merge': return 'Diverged — manual merge required';
      case 'changes': return `${s.n} uncommitted change${s.n === 1 ? '' : 's'}`;
      case 'ahead': return `${s.n} commit${s.n === 1 ? '' : 's'} to push`;
      case 'behind': return `${s.n} commit${s.n === 1 ? '' : 's'} to pull`;
      case 'clean': return 'Up to date';
      default: return '';
    }
  }
</script>

{#if state}
  <span class="flex items-center shrink-0 px-1.5" title={label}>
    {#if state.kind === 'op'}
      <span class="relative inline-flex items-center justify-center w-4 h-4">
        <svg class="absolute inset-0 animate-spin text-fg-subtle" viewBox="0 0 16 16" fill="none">
          <circle cx="8" cy="8" r="6" stroke="currentColor" stroke-width="2" opacity="0.3" />
          <path d="M14 8a6 6 0 0 0-6-6" stroke="currentColor" stroke-width="2" stroke-linecap="round" />
        </svg>
        <span class="text-[9px] font-bold leading-none {OP_COLOR[state.op]}">
          {OP_GLYPH[state.op]}
        </span>
      </span>
    {:else if state.kind === 'error'}
      <svg class="w-4 h-4 text-danger" viewBox="0 0 16 16" fill="none">
        <path d="M4.5 4.5l7 7M11.5 4.5l-7 7" stroke="currentColor"
              stroke-width="2" stroke-linecap="round" />
      </svg>
    {:else if state.kind === 'merge'}
      <svg class="w-4 h-4 text-attention" viewBox="0 0 16 16" fill="none">
        <path d="M8 2.5 14.5 13.5H1.5L8 2.5Z" stroke="currentColor"
              stroke-width="1.4" stroke-linejoin="round" />
        <path d="M8 6.3v3.1" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" />
        <circle cx="8" cy="11.6" r="0.85" fill="currentColor" />
      </svg>
    {:else if state.kind === 'clean'}
      <svg class="w-4 h-4 text-success" viewBox="0 0 16 16" fill="none">
        <path d="M3.5 8.5 6.5 11.5 12.5 5" stroke="currentColor"
              stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
    {:else}
      <span
        class="inline-flex items-center gap-0.5 text-xs font-semibold leading-none
               {state.kind === 'changes' ? 'text-attention' : 'text-accent'}"
      >
        <span>{state.kind === 'changes' ? '●' : state.kind === 'ahead' ? '↑' : '↓'}</span>
        <span>{state.n}</span>
      </span>
    {/if}
  </span>
{/if}
