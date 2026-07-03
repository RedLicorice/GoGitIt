<script>
  import { onMount } from 'svelte';
  import { user, loadMe, loadRepos, loadMcp, selectedRepo, sidebarOpen } from './lib/stores.js';
  import { refreshRepoStatuses } from './lib/repostatus.js';
  import { startLive } from './lib/live.js';
  import Sidebar from './components/Sidebar.svelte';
  import TopBar from './components/TopBar.svelte';
  import RepoView from './components/RepoView.svelte';
  import EmptyState from './components/EmptyState.svelte';
  import SettingsModal from './components/SettingsModal.svelte';
  import ToastContainer from './components/ToastContainer.svelte';
  import DivergeDialog from './components/DivergeDialog.svelte';

  let loading = true;
  let error = null;

  onMount(async () => {
    try {
      await loadMe();
      await loadRepos();
      refreshRepoStatuses();
      loadMcp();
      startLive();
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  });
</script>

<div class="h-[100dvh] flex flex-col bg-canvas overflow-hidden">
  <TopBar />

  {#if loading}
    <div class="flex-1 flex items-center justify-center text-fg-muted text-sm">
      Loading…
    </div>
  {:else if error}
    <div class="flex-1 flex items-center justify-center text-danger text-sm">
      {error}
    </div>
  {:else}
    <div class="flex-1 flex overflow-hidden">
      {#if $sidebarOpen}
        <Sidebar />
      {/if}
      <main class="flex-1 flex flex-col overflow-hidden">
        {#if $selectedRepo}
          <RepoView repo={$selectedRepo} />
        {:else}
          <EmptyState />
        {/if}
      </main>
    </div>
  {/if}

  <SettingsModal />
  <DivergeDialog />
  <ToastContainer />
</div>
