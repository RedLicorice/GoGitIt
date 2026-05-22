<script>
  import { diffMode } from '../lib/stores.js';
</script>

<div class="inline-flex rounded-md border border-border overflow-hidden shrink-0">
  <button
    class="px-2 py-0.5 text-xs transition-colors
           {$diffMode === 'inline'
             ? 'bg-accent-subtle text-fg'
             : 'text-fg-muted hover:text-fg'}"
    on:click={() => diffMode.set('inline')}
  >Unified</button>
  <button
    class="px-2 py-0.5 text-xs border-l border-border transition-colors
           {$diffMode === 'split'
             ? 'bg-accent-subtle text-fg'
             : 'text-fg-muted hover:text-fg'}"
    on:click={() => diffMode.set('split')}
  >Split</button>
</div>
