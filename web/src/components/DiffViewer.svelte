<script>
  import { diffMode } from '../lib/stores.js';

  // A FileDiff: { path, status, binary, additions, deletions, hunks: [...] }.
  export let file;

  // Pair up delete/add runs for side-by-side rendering. Context lines map to
  // both columns; a run of N deletes and M adds yields max(N,M) rows, with the
  // shorter side padded by nulls.
  function splitRows(lines) {
    const rows = [];
    let i = 0;
    while (i < lines.length) {
      const l = lines[i];
      if (l.type === 'context') {
        rows.push({ left: l, right: l });
        i++;
        continue;
      }
      const dels = [];
      const adds = [];
      while (i < lines.length && lines[i].type === 'delete') dels.push(lines[i++]);
      while (i < lines.length && lines[i].type === 'add') adds.push(lines[i++]);
      const n = Math.max(dels.length, adds.length);
      for (let k = 0; k < n; k++) {
        rows.push({ left: dels[k] || null, right: adds[k] || null });
      }
    }
    return rows;
  }

  function rowBg(type) {
    if (type === 'add') return 'bg-success/10';
    if (type === 'delete') return 'bg-danger/10';
    return '';
  }
  function signChar(type) {
    if (type === 'add') return '+';
    if (type === 'delete') return '-';
    return ' ';
  }
  function signColor(type) {
    if (type === 'add') return 'text-success';
    if (type === 'delete') return 'text-danger';
    return 'text-fg-subtle';
  }
</script>

{#if file}
  {#if file.binary}
    <div class="px-3 py-3 text-sm text-fg-muted italic">Binary file — diff not shown.</div>
  {:else if !file.hunks || file.hunks.length === 0}
    <div class="px-3 py-3 text-sm text-fg-muted italic">No textual changes.</div>
  {:else if $diffMode === 'inline'}
    <!-- Unified / inline mode -->
    <div class="overflow-x-auto text-[12px] leading-5 font-mono">
      {#each file.hunks as hunk}
        <div class="px-2 py-0.5 bg-accent-subtle text-fg-muted select-none whitespace-pre">
          {hunk.header}
        </div>
        {#each hunk.lines as line}
          <div class="flex min-w-full w-max {rowBg(line.type)}">
            <div class="sticky left-0 z-10 flex shrink-0 bg-canvas">
              <span class="w-12 px-2 text-right text-fg-subtle select-none">{line.old_line || ''}</span>
              <span class="w-12 px-2 text-right text-fg-subtle select-none">{line.new_line || ''}</span>
              <span class="w-5 text-center select-none {signColor(line.type)}">{signChar(line.type)}</span>
            </div>
            <span class="whitespace-pre px-2">{line.content || ' '}</span>
          </div>
        {/each}
      {/each}
    </div>
  {:else}
    <!-- Side-by-side / split mode -->
    <div class="text-[12px] leading-5 font-mono">
      {#each file.hunks as hunk}
        {@const rows = splitRows(hunk.lines)}
        <div class="px-2 py-0.5 bg-accent-subtle text-fg-muted select-none whitespace-pre">
          {hunk.header}
        </div>
        <div class="flex">
          <!-- old side -->
          <div class="w-1/2 overflow-x-auto border-r border-border">
            {#each rows as row}
              <div class="flex min-w-full w-max {row.left ? rowBg(row.left.type) : 'bg-canvas-inset'}">
                <span class="sticky left-0 z-10 w-12 shrink-0 px-2 text-right text-fg-subtle bg-canvas select-none">
                  {row.left?.old_line || ''}
                </span>
                <span class="whitespace-pre px-2">{row.left ? row.left.content || ' ' : ' '}</span>
              </div>
            {/each}
          </div>
          <!-- new side -->
          <div class="w-1/2 overflow-x-auto">
            {#each rows as row}
              <div class="flex min-w-full w-max {row.right ? rowBg(row.right.type) : 'bg-canvas-inset'}">
                <span class="sticky left-0 z-10 w-12 shrink-0 px-2 text-right text-fg-subtle bg-canvas select-none">
                  {row.right?.new_line || ''}
                </span>
                <span class="whitespace-pre px-2">{row.right ? row.right.content || ' ' : ' '}</span>
              </div>
            {/each}
          </div>
        </div>
      {/each}
    </div>
  {/if}
{/if}
