<script>
  import { user, selectedRepo, sidebarOpen, settingsOpen, mcpState } from '../lib/stores.js';
</script>

<header class="h-12 shrink-0 flex items-center justify-between px-4
               border-b border-border bg-canvas-subtle sticky top-0 z-40">
  <div class="flex items-center gap-3 min-w-0">
    <button
      class="shrink-0 text-fg-muted hover:text-fg transition-colors p-1 -ml-1 rounded
             {$sidebarOpen ? '' : 'text-fg'}"
      title={$sidebarOpen ? 'Hide repositories' : 'Show repositories'}
      aria-label="Toggle repositories sidebar"
      on:click={() => sidebarOpen.update((v) => !v)}
    >
      <svg viewBox="0 0 16 16" class="w-5 h-5" fill="currentColor" aria-hidden="true">
        <path d="M1 3.75A.75.75 0 0 1 1.75 3h12.5a.75.75 0 0 1 0 1.5H1.75A.75.75 0 0 1 1 3.75Zm0 4A.75.75 0 0 1 1.75 7h12.5a.75.75 0 0 1 0 1.5H1.75A.75.75 0 0 1 1 7.75ZM1.75 11h12.5a.75.75 0 0 1 0 1.5H1.75a.75.75 0 0 1 0-1.5Z"/>
      </svg>
    </button>
    <div class="flex items-center gap-2 text-fg font-semibold tracking-tight shrink-0">
      <svg viewBox="0 0 24 24" class="w-5 h-5 text-accent" aria-hidden="true">
        <g stroke="currentColor" stroke-width="2" stroke-linecap="round" fill="none">
          <path d="M8 7v10"/>
          <path d="M8 10.5c0 3.5 2 4.5 5.5 4.5"/>
        </g>
        <g fill="currentColor">
          <circle cx="8" cy="5.5" r="2.6"/>
          <circle cx="8" cy="18.5" r="2.6"/>
          <circle cx="16" cy="15" r="2.6"/>
        </g>
      </svg>
      <span class="hidden sm:inline">GoGitIt</span>
    </div>
    {#if $selectedRepo}
      <span class="text-fg-muted text-sm shrink-0">/</span>
      <span class="text-fg text-sm font-medium truncate">{$selectedRepo.name}</span>
    {/if}
  </div>

  <div class="flex items-center gap-3 shrink-0">
    {#if $mcpState}
      <button
        class="inline-flex items-center gap-1.5 px-1.5 py-0.5 rounded
               text-[11px] font-mono text-fg-muted hover:bg-border-muted transition-colors"
        title={$mcpState.enabled ? 'MCP server: enabled — click to configure' : 'MCP server: disabled — click to configure'}
        aria-label="MCP server status"
        on:click={() => settingsOpen.set(true)}
      >
        <span class="block w-1.5 h-1.5 rounded-full {$mcpState.enabled ? 'bg-success' : 'bg-fg-muted'}"></span>
        <span class="hidden sm:inline">MCP</span>
      </button>
    {/if}
    <button
      class="text-fg-muted hover:text-fg transition-colors p-1 rounded"
      title="Settings"
      aria-label="Open settings"
      on:click={() => settingsOpen.set(true)}
    >
      <svg viewBox="0 0 16 16" class="w-5 h-5" fill="currentColor" aria-hidden="true">
        <path d="M8 0a8.2 8.2 0 0 1 .701.031C9.444.095 9.99.645 10.16 1.29l.288 1.107c.018.066.078.158.211.224.327.157.642.36.929.598.108.085.291.121.448.082l1.105-.275c.667-.166 1.43.108 1.79.781.117.21.227.424.327.633.39.7.045 1.535-.474 2.025l-.7.633a3.5 3.5 0 0 1 0 1.21l.7.633c.519.49.864 1.325.474 2.025-.1.21-.21.424-.327.633-.36.673-1.123.947-1.79.781l-1.105-.275c-.157-.039-.34-.003-.448.082-.287.238-.602.44-.929.598-.133.066-.193.158-.211.224l-.288 1.107c-.17.645-.716 1.195-1.459 1.26a8.16 8.16 0 0 1-1.402 0c-.743-.065-1.289-.615-1.459-1.26l-.288-1.107c-.018-.066-.078-.158-.211-.224a6.992 6.992 0 0 1-.929-.598c-.108-.085-.291-.121-.448-.082l-1.105.275c-.667.166-1.43-.108-1.79-.781a8.04 8.04 0 0 1-.327-.633c-.39-.7-.045-1.535.474-2.025l.7-.633a3.5 3.5 0 0 1 0-1.21l-.7-.633C.635 6.155.29 5.32.68 4.62c.1-.21.21-.424.327-.633.36-.673 1.123-.947 1.79-.781l1.105.275c.157.039.34.003.448-.082.287-.238.602-.44.929-.598.133-.066.193-.158.211-.224L6.79 1.29C6.96.645 7.506.095 8.249.03 8.4.01 8.7 0 8 0Zm0 5.5a2.5 2.5 0 1 0 0 5 2.5 2.5 0 0 0 0-5Z"/>
      </svg>
    </button>
    {#if $user}
      <span class="text-fg-muted text-sm">
        {$user.username || $user.name || $user.subject}
      </span>
      {#if $user.subject !== 'local'}
        <a
          href="/auth/logout"
          class="text-fg-muted hover:text-fg transition-colors p-1 rounded"
          title="Logout"
          aria-label="Logout"
        >
          <svg viewBox="0 0 16 16" class="w-5 h-5" fill="currentColor" aria-hidden="true">
            <path d="M2 2.75A.75.75 0 0 1 2.75 2h6.5a.75.75 0 0 1 0 1.5h-5.75v9h5.75a.75.75 0 0 1 0 1.5h-6.5a.75.75 0 0 1-.75-.75ZM12.193 8H6.75a.75.75 0 0 1 0-1.5h5.443L10.22 4.53a.75.75 0 1 1 1.06-1.06l3.25 3.25a.75.75 0 0 1 0 1.06l-3.25 3.25a.75.75 0 1 1-1.06-1.06L12.193 8Z"/>
          </svg>
        </a>
      {/if}
    {/if}
  </div>
</header>
