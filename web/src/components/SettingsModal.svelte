<script>
  import { settingsOpen } from '../lib/stores.js';
  import { api } from '../lib/api.js';

  let loading = false;
  let saving = false;
  let error = null;
  let data = null;

  let name = '';
  let email = '';
  let pat = ''; // new token to save; empty = leave the stored one unchanged

  // Transient "saved" feedback.
  let notice = null;
  let noticeTimer;
  function flashNotice(msg) {
    notice = msg;
    clearTimeout(noticeTimer);
    noticeTimer = setTimeout(() => (notice = null), 3000);
  }

  // Load settings each time the modal transitions to open.
  let lastOpen = false;
  $: if ($settingsOpen !== lastOpen) {
    lastOpen = $settingsOpen;
    if ($settingsOpen) loadSettings();
  }

  async function loadSettings() {
    loading = true;
    error = null;
    try {
      data = await api.getSettings();
      name = data.identity?.name || '';
      email = data.identity?.email || '';
      pat = '';
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function save() {
    saving = true;
    error = null;
    try {
      const payload = { identity: { name, email } };
      if (pat) payload.github_pat = pat;
      data = await api.saveSettings(payload);
      pat = '';
      flashNotice('Settings saved!');
    } catch (e) {
      error = e.message;
    } finally {
      saving = false;
    }
  }

  async function removeToken() {
    saving = true;
    error = null;
    try {
      data = await api.saveSettings({ identity: { name, email }, github_pat: '' });
      pat = '';
      flashNotice('Saved token removed.');
    } catch (e) {
      error = e.message;
    } finally {
      saving = false;
    }
  }

  function close() {
    settingsOpen.set(false);
  }
  function onKey(e) {
    if (e.key === 'Escape' && $settingsOpen) close();
  }
</script>

<svelte:window on:keydown={onKey} />

{#if $settingsOpen}
  <div class="fixed inset-0 z-50 flex items-center justify-center">
    <button
      class="absolute inset-0 bg-black/60 cursor-default"
      aria-label="Close settings"
      on:click={close}
    ></button>

    <div
      class="relative w-full max-w-lg mx-4 rounded-lg border border-border
             bg-canvas-subtle shadow-2xl max-h-[85vh] flex flex-col"
      role="dialog"
      aria-modal="true"
      aria-label="Settings"
    >
      <div class="px-4 py-3 border-b border-border flex items-center justify-between">
        <h3 class="text-sm font-semibold text-fg">Settings</h3>
        <button
          class="text-fg-muted hover:text-fg text-xl leading-none"
          aria-label="Close"
          on:click={close}
        >×</button>
      </div>

      <div class="flex-1 overflow-y-auto p-4 space-y-6">
        {#if loading}
          <p class="text-sm text-fg-muted">Loading…</p>
        {:else if error}
          <p class="text-sm text-danger">{error}</p>
        {:else if data}
          <!-- Git identity -->
          <section class="space-y-3">
            <div>
              <h4 class="text-xs uppercase tracking-wider text-fg-muted font-semibold">
                Git Identity
              </h4>
              <p class="text-xs text-fg-muted mt-1">
                Stamped as the author of commits you create in GoGitIt.
              </p>
            </div>
            <label class="block text-xs text-fg-muted">
              Name
              <input class="input mt-1" bind:value={name} placeholder="Ada Lovelace" />
            </label>
            <label class="block text-xs text-fg-muted">
              Email
              <input class="input mt-1" type="email" bind:value={email}
                     placeholder="ada@example.com" />
            </label>
          </section>

          <!-- GitHub authentication -->
          <section class="space-y-3">
            <div>
              <h4 class="text-xs uppercase tracking-wider text-fg-muted font-semibold">
                GitHub Authentication
              </h4>
              <p class="text-xs text-fg-muted mt-1">
                Credentials for HTTPS push / pull / fetch against GitHub.
              </p>
            </div>

            <div class="rounded-md border border-border bg-canvas-inset px-3 py-2 text-sm">
              {#if !data.github.gh.enabled}
                <span class="text-fg-muted">gh CLI integration disabled</span>
                <p class="text-xs text-fg-muted mt-1">
                  Turned off by configuration (<code class="font-mono">gh.enabled</code>).
                  Use a token below.
                </p>
              {:else if data.github.gh.available && data.github.gh.authenticated}
                <span class="text-success">✓ gh CLI detected and authenticated</span>
                <p class="text-xs text-fg-muted mt-1">
                  GoGitIt can reuse your <code class="font-mono">gh</code> login — no token needed.
                </p>
              {:else if data.github.gh.available}
                <span class="text-attention">gh CLI detected, but not logged in</span>
                <p class="text-xs text-fg-muted mt-1">
                  Run <code class="font-mono">gh auth login</code>, or set a token below.
                </p>
              {:else}
                <span class="text-fg-muted">gh CLI not found</span>
                <p class="text-xs text-fg-muted mt-1">
                  Install GitHub CLI to skip manual tokens, or set a token below.
                </p>
              {/if}
            </div>

            <label class="block text-xs text-fg-muted">
              Personal Access Token
              <input
                class="input mt-1"
                type="password"
                bind:value={pat}
                placeholder={data.github.pat_set
                  ? '•••••••••••• (a token is saved)'
                  : 'ghp_…'}
              />
            </label>
            <div class="flex items-center justify-between">
              <p class="text-[11px] text-fg-muted">
                Stored locally in <code class="font-mono">settings.json</code> (0600).
              </p>
              {#if data.github.pat_set}
                <button class="btn-mini hover:!text-danger" disabled={saving}
                        on:click={removeToken}>Remove saved token</button>
              {/if}
            </div>
          </section>

          <!-- About -->
          <section class="space-y-1">
            <h4 class="text-xs uppercase tracking-wider text-fg-muted font-semibold">
              About
            </h4>
            <div class="text-xs font-mono text-fg-muted leading-5 pt-1">
              <div>gogitit&nbsp;&nbsp;<span class="text-fg">{data.about.version}</span></div>
              <div>go-git&nbsp;&nbsp;&nbsp;<span class="text-fg">{data.about.go_git || '—'}</span></div>
              <div>runtime&nbsp;&nbsp;<span class="text-fg">{data.about.go}</span></div>
            </div>
          </section>
        {/if}
      </div>

      <div class="px-4 py-3 border-t border-border flex items-center justify-between gap-2">
        <span class="text-xs text-success">{notice || ''}</span>
        <div class="flex gap-2">
          <button class="btn" on:click={close}>Close</button>
          <button class="btn btn-primary" on:click={save} disabled={saving || loading}>
            {saving ? 'Saving…' : 'Save'}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
