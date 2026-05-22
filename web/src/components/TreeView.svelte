<script>
  // ungit-style commit graph. The backend returns commits topologically sorted
  // (children before parents); this assigns each a lane and draws the rail.
  import { createEventDispatcher } from 'svelte';
  import { api } from '../lib/api.js';
  import { addToast } from '../lib/toasts.js';

  export let repo;

  const dispatch = createEventDispatcher();

  const ROW_H = 64;
  const LANE_W = 18;
  const DOT_R = 5;
  // Lane colours cycle; listed as literals so Tailwind keeps the classes.
  const STROKE = ['stroke-accent', 'stroke-success', 'stroke-attention', 'stroke-danger', 'stroke-fg-muted'];
  const FILL = ['fill-accent', 'fill-success', 'fill-attention', 'fill-danger', 'fill-fg-muted'];

  let graph = null;
  let loading = true;
  let error = null;
  let rows = [];
  let maxLanes = 1;
  let refsByHash = {};
  let lastRepoId = null;

  // "Attach branch" to an abandoned commit.
  let attachHash = null;
  let attachShort = '';
  let attachName = '';
  let attaching = false;

  $: if (repo && repo.id !== lastRepoId) {
    lastRepoId = repo.id;
    load();
  }

  async function load() {
    loading = true;
    error = null;
    graph = null;
    try {
      graph = await api.graph(repo.id);
      buildLayout(graph);
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  function openAttach(c) {
    attachHash = c.hash;
    attachShort = c.short_hash;
    attachName = '';
  }
  async function attach() {
    const name = attachName.trim();
    if (!name) return;
    attaching = true;
    try {
      await api.createBranch(repo.id, name, attachHash);
      addToast({ kind: 'success', message: `Branch ${name} attached to ${attachShort}` });
      attachHash = null;
      dispatch('changed'); // RepoView reloads branches + remounts this view
    } catch (e) {
      addToast({ kind: 'error', message: e.message, timeout: 9000 });
    } finally {
      attaching = false;
    }
  }

  // Assign each commit a lane. lanes[i] holds the hash lane i is waiting for.
  function buildLayout(g) {
    refsByHash = {};
    for (const ref of g.refs || []) {
      (refsByHash[ref.hash] ||= []).push(ref);
    }
    const lanes = [];
    const out = [];
    let mx = 1;
    for (const commit of g.commits) {
      let myLane = lanes.indexOf(commit.hash);
      if (myLane === -1) {
        myLane = lanes.indexOf(null);
        if (myLane === -1) myLane = lanes.length;
      }
      const before = lanes.slice();

      lanes[myLane] = null;
      for (let i = 0; i < lanes.length; i++) {
        if (lanes[i] === commit.hash) lanes[i] = null; // branches converging in
      }

      const parentLanes = [];
      (commit.parents || []).forEach((p, k) => {
        if (k === 0) {
          lanes[myLane] = p;
          parentLanes.push(myLane);
        } else {
          let pl = lanes.indexOf(p);
          if (pl === -1) {
            pl = lanes.indexOf(null);
            if (pl === -1) pl = lanes.length;
            lanes[pl] = p;
          }
          parentLanes.push(pl);
        }
      });

      mx = Math.max(mx, before.length, lanes.length, myLane + 1);
      out.push({ commit, lane: myLane, before, parentLanes });
    }
    rows = out;
    maxLanes = mx;
  }

  const laneX = (i) => i * LANE_W + LANE_W / 2;

  function boxClass(c) {
    if (!c.reachable) return 'border-danger/50 border-dashed opacity-60';
    if (graph && c.hash === graph.head_hash) return 'border-accent';
    if (!c.pushed) return 'border-border border-l-2 border-l-attention';
    return 'border-border';
  }
  function refChip(ref) {
    if (ref.type === 'branch') return 'bg-accent-subtle text-accent';
    if (ref.type === 'remote') return 'bg-border-muted text-fg-muted';
    return 'bg-attention/15 text-attention'; // tag
  }
  function fmtDate(d) {
    return new Date(d).toLocaleString(undefined, {
      year: '2-digit', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
    });
  }
</script>

<div class="h-full overflow-auto">
  {#if loading}
    <p class="px-4 py-4 text-sm text-fg-muted">Loading graph…</p>
  {:else if error}
    <p class="px-4 py-4 text-sm text-danger">{error}</p>
  {:else if !graph || graph.commits.length === 0}
    <p class="px-4 py-4 text-sm text-fg-muted">No commits.</p>
  {:else}
    {#if graph.truncated}
      <div class="px-4 py-1.5 text-xs text-attention bg-attention/10 border-b border-border">
        Graph capped at {graph.commits.length} commits — older commits not shown.
      </div>
    {/if}
    <div class="flex" style="min-height: {rows.length * ROW_H}px">
      <svg
        class="shrink-0"
        width={maxLanes * LANE_W}
        height={rows.length * ROW_H}
      >
        {#each rows as row, k}
          {@const y0 = k * ROW_H}
          {@const mid = y0 + ROW_H / 2}
          {@const y1 = y0 + ROW_H}
          {@const xd = laneX(row.lane)}
          {#each row.before as h, i}
            {#if h}
              <line
                x1={laneX(i)} y1={y0}
                x2={h === row.commit.hash ? xd : laneX(i)}
                y2={h === row.commit.hash ? mid : y1}
                class={STROKE[i % STROKE.length]}
                stroke-width="2" stroke-linecap="round"
              />
            {/if}
          {/each}
          {#each row.parentLanes as pl}
            <line
              x1={xd} y1={mid} x2={laneX(pl)} y2={y1}
              class={STROKE[pl % STROKE.length]}
              stroke-width="2" stroke-linecap="round"
            />
          {/each}
          <circle
            cx={xd} cy={mid} r={DOT_R}
            class="{row.commit.hash === graph.head_hash
              ? 'fill-accent'
              : FILL[row.lane % FILL.length]} stroke-canvas"
            stroke-width="2"
          />
        {/each}
      </svg>

      <div class="flex-1 min-w-0 pr-3">
        {#each rows as row}
          {@const c = row.commit}
          <div class="flex flex-col justify-center pl-1 pr-2" style="height: {ROW_H}px">
            <div class="rounded-md border px-2.5 py-1.5 bg-canvas-subtle {boxClass(c)}">
              <div class="flex items-center gap-1.5">
                {#each refsByHash[c.hash] || [] as ref}
                  <span class="text-[10px] leading-none px-1 py-0.5 rounded font-mono shrink-0 {refChip(ref)}">
                    {ref.name}
                  </span>
                {/each}
                {#if c.hash === graph.head_hash}
                  <span class="text-[10px] leading-none px-1 py-0.5 rounded font-mono shrink-0 bg-accent text-white">
                    HEAD
                  </span>
                {/if}
                <span class="text-sm text-fg truncate flex-1 min-w-0">{c.summary}</span>
                {#if !c.reachable}
                  <button
                    class="btn-mini shrink-0"
                    on:click={() => openAttach(c)}
                  >Attach branch</button>
                {/if}
              </div>
              <div class="text-xs text-fg-muted mt-0.5 flex items-center gap-1.5">
                <span class="font-mono shrink-0">{c.short_hash}</span>
                <span class="shrink-0">·</span>
                <span class="truncate">{c.author}</span>
                <span class="shrink-0">·</span>
                <span class="shrink-0">{fmtDate(c.date)}</span>
                {#if !c.reachable}
                  <span class="shrink-0 text-danger">· abandoned</span>
                {:else if !c.pushed}
                  <span class="shrink-0 text-attention">· unpushed</span>
                {/if}
              </div>
            </div>
          </div>
        {/each}
      </div>
    </div>
  {/if}
</div>

{#if attachHash}
  <div class="fixed inset-0 z-50 flex items-center justify-center">
    <button
      class="absolute inset-0 bg-black/60 cursor-default"
      aria-label="Cancel"
      on:click={() => (attachHash = null)}
    ></button>
    <div
      class="relative w-full max-w-sm mx-4 rounded-lg border border-border
             bg-canvas-subtle shadow-2xl"
      role="dialog"
      aria-modal="true"
    >
      <div class="px-4 py-3 border-b border-border">
        <h3 class="text-sm font-semibold text-fg">Attach a branch</h3>
      </div>
      <div class="p-4 space-y-2">
        <p class="text-xs text-fg-muted">
          Create a branch at abandoned commit
          <span class="font-mono text-fg">{attachShort}</span>
          to make it reachable again — then switch, merge or push it from the
          branch menu.
        </p>
        <input
          class="input"
          placeholder="branch name"
          bind:value={attachName}
          disabled={attaching}
          on:keydown={(e) => e.key === 'Enter' && attach()}
        />
      </div>
      <div class="px-4 py-3 border-t border-border flex justify-end gap-2">
        <button class="btn" on:click={() => (attachHash = null)}>Cancel</button>
        <button
          class="btn btn-primary"
          disabled={attaching || !attachName.trim()}
          on:click={attach}
        >{attaching ? 'Attaching…' : 'Attach branch'}</button>
      </div>
    </div>
  </div>
{/if}
