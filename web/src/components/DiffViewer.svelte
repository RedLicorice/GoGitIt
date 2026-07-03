<script>
  import { createEventDispatcher } from 'svelte';
  import { api } from '../lib/api.js';
  import { diffMode, repos, selectedRepo, selectedRepoId } from '../lib/stores.js';
  import { addToast } from '../lib/toasts.js';
  import ShaPill from './ShaPill.svelte';

  const dispatch = createEventDispatcher();

  // A FileDiff: { path, status, binary, additions, deletions, hunks: [...],
  //   is_submodule?, submodule_old?, submodule_new?, submodule_log? }.
  export let file;
  // Repo + optional commit hash for resolving raw image previews. `commit`
  // unset → worktree; set → blob at that commit's tree.
  export let repoId = null;
  export let commit = null;

  const IMG_EXTS = new Set([
    'png', 'jpg', 'jpeg', 'gif', 'webp', 'avif', 'svg', 'ico', 'bmp',
  ]);
  function imageExt(p) {
    const dot = (p || '').lastIndexOf('.');
    if (dot < 0) return '';
    return p.slice(dot + 1).toLowerCase();
  }
  $: isImage = file && file.binary && IMG_EXTS.has(imageExt(file.path));
  $: imageUrl =
    isImage && repoId ? api.rawUrl(repoId, file.path, commit || undefined) : null;

  // Registered submodule repo (if any) matching this entry. Drives the
  // "Open in GoGitIt" affordance.
  $: subRepo =
    file?.is_submodule && $selectedRepo
      ? $repos.find(
          (r) =>
            r.parent_id === $selectedRepo.id &&
            r.path.replace(/\/$/, '').endsWith('/' + file.path.replace(/\/$/, '')),
        )
      : null;

  let working = false;
  async function commitPushRef() {
    if (!$selectedRepo) return;
    working = true;
    try {
      const res = await api.submoduleCommitPush($selectedRepo.id, file.path);
      addToast({
        kind: 'success',
        message: `Updated submodule ${file.path} — committed and pushed to ${res.remote}`,
      });
      dispatch('committed', { path: file.path });
    } catch (e) {
      addToast({ kind: 'error', message: e.message, timeout: 9000 });
    } finally {
      working = false;
    }
  }
  function openSubmodule() {
    if (subRepo) selectedRepoId.set(subRepo.id);
  }
  function firstLine(s) {
    return (s || '').split('\n')[0];
  }

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
    {#if imageUrl}
      <div class="p-3 md:p-6 flex justify-center bg-canvas-subtle">
        <img
          src={imageUrl}
          alt={file.path}
          class="max-w-full max-h-[70vh] object-contain rounded border border-border bg-canvas"
        />
      </div>
    {:else}
      <div class="px-3 py-3 text-sm text-fg-muted italic">Binary file — diff not shown.</div>
    {/if}
  {:else if file.is_submodule}
    <!-- Submodule (gitlink) entry — show the SHAs and a log of commits the
         submodule moved through; offer to open the submodule (if registered)
         and to commit + push the gitlink change in this parent. -->
    <div class="p-3 md:p-6 space-y-4">
      <div class="flex items-center gap-2 flex-wrap">
        <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded
                     bg-attention/15 text-attention font-semibold">
          Submodule
        </span>
        <span class="text-sm font-mono text-fg-muted truncate">{file.path}</span>
      </div>

      <div class="flex items-center gap-2 text-xs flex-wrap">
        {#if file.submodule_old}
          <ShaPill sha={file.submodule_old} />
        {:else}
          <span class="text-fg-muted italic">(none)</span>
        {/if}
        <span class="text-fg-muted">→</span>
        {#if file.submodule_new}
          <ShaPill sha={file.submodule_new} />
        {:else}
          <span class="text-fg-muted italic">(none)</span>
        {/if}
      </div>

      {#if file.submodule_log && file.submodule_log.length}
        <div class="space-y-1.5">
          <p class="text-xs uppercase tracking-wider text-fg-muted font-semibold">
            {file.submodule_log.length} commit{file.submodule_log.length === 1 ? '' : 's'} in the submodule
          </p>
          <ul class="border border-border rounded divide-y divide-border bg-canvas-subtle">
            {#each file.submodule_log as c (c.hash)}
              <li class="px-2.5 py-1.5">
                <div class="flex items-center gap-2 min-w-0">
                  <ShaPill sha={c.hash} label={c.short_hash} />
                  <span class="text-sm text-fg truncate">{firstLine(c.message)}</span>
                </div>
                <div class="text-[11px] text-fg-muted mt-0.5 truncate">{c.author}</div>
              </li>
            {/each}
          </ul>
        </div>
      {:else if file.submodule_old && file.submodule_new}
        <p class="text-xs text-fg-muted italic">
          Cannot read the submodule's history locally — log not available.
        </p>
      {/if}

      <div class="flex flex-wrap gap-2 pt-1">
        {#if subRepo}
          <button class="btn" on:click={openSubmodule}>
            Open {subRepo.name} in GoGitIt
          </button>
        {/if}
        <button class="btn btn-primary" disabled={working} on:click={commitPushRef}>
          {working ? 'Committing…' : 'Commit & push reference'}
        </button>
      </div>
    </div>
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
