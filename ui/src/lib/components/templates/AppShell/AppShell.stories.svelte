<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import AppShell from './AppShell.svelte';
  import EmptyState from '../../molecules/EmptyState/EmptyState.svelte';
  import Banner from '../../molecules/Banner/Banner.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import { ToastQueue } from '../../../toast/toasts.svelte';
  import type { PaletteItem } from '../../organisms/CommandPalette/CommandPalette.svelte';

  const status = { unhealthy: 1, healthTone: 'error' as const, active: 4, blocked: 1 };
  const items: PaletteItem[] = [
    { id: 'tlh', label: 'The Long Haul', group: 'Items', kind: 'tv', subtitle: 'TV · 2024' },
    { id: 'dock', label: 'Dockside', group: 'Items', kind: 'movie', subtitle: 'Movie · 2026' },
    { id: 'pause-all', label: 'Pause all downloads', group: 'Actions', icon: 'pause' },
  ];
  const toastQueue = new ToastQueue();
  toastQueue.push({ message: 'Killed Dockside and blocklisted the release', detail: 'Searching for another release.', onundo: () => {}, duration: 0 });
  const fullscreen = { layout: 'fullscreen' };
  const phone = { viewport: { value: 'phone', isRotated: false } };
  const desktop = { viewport: { value: 'desktop', isRotated: false } };

  const { Story } = defineMeta({
    title: 'Templates/AppShell',
    component: AppShell,
    tags: ['autodocs'],
    parameters: fullscreen,
    args: { kinds: ['tv', 'movie', 'music'], path: '/library/movies', status, connection: 'live' },
  });
</script>

{#snippet body()}
  <h1 style="font-size: var(--text-xl); margin-bottom: var(--space-6)">Movies</h1>
  <EmptyState title="Screen content goes here" level={2}>The shell frames every authenticated screen; routes render inside main.</EmptyState>
{/snippet}

<Story name="Playground" globals={desktop}>
  {#snippet template(args)}<AppShell {...args} paletteItems={items}>{@render body()}</AppShell>{/snippet}
</Story>

<Story name="Desktop light" asChild globals={{ ...desktop, theme: 'light' }}>
  <AppShell kinds={['tv', 'movie', 'music']} path="/library/movies" {status} paletteItems={items} onsignout={() => {}}>{@render body()}</AppShell>
</Story>

<Story name="Desktop dark" asChild globals={{ ...desktop, theme: 'dark' }}>
  <AppShell kinds={['tv', 'movie', 'music']} path="/library/movies" {status} paletteItems={items} onsignout={() => {}}>{@render body()}</AppShell>
</Story>

<Story name="Phone light" asChild globals={{ ...phone, theme: 'light' }}>
  <AppShell kinds={['tv', 'movie']} path="/activity/queue" {status}>{@render body()}</AppShell>
</Story>

<Story name="Phone dark" asChild globals={{ ...phone, theme: 'dark' }}>
  <AppShell kinds={['tv', 'movie']} path="/activity/queue" {status}>{@render body()}</AppShell>
</Story>

<Story name="Reconnecting" asChild globals={desktop}>
  <AppShell path="/" status={{ ...status, unhealthy: 0 }} connection="reconnecting">{@render body()}</AppShell>
</Story>

<Story name="Offline, phone" asChild globals={phone}>
  <AppShell path="/" status={{ ...status, unhealthy: 0 }} connection="offline" offlineSince="14:05" onreconnect={() => {}}>{@render body()}</AppShell>
</Story>

<Story name="Shadow mode and a custom banner" asChild globals={desktop}>
  <AppShell kinds={['tv', 'movie', 'music']} path="/system/health" status={{ ...status, shadowKinds: ['Music'] }}>
    {#snippet banner()}
      <Banner preset="restart" title="Restart needed">The listen address changed in the config file; it takes effect after a restart.{#snippet actions()}<Button size="sm">Restart now</Button>{/snippet}</Banner>
    {/snippet}
    {@render body()}
  </AppShell>
</Story>

<Story name="Command palette open" asChild globals={desktop}>
  <AppShell path="/" {status} paletteItems={items} paletteRecent={['dock']} paletteOpen>{@render body()}</AppShell>
</Story>

<Story name="With a toast" asChild globals={phone}>
  <AppShell path="/activity/queue" {status} toasts={toastQueue}>{@render body()}</AppShell>
</Story>
