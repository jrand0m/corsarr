<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import CommandPalette, { type PaletteItem } from './CommandPalette.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import Kbd from '../../atoms/Kbd/Kbd.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const items: PaletteItem[] = [
    { id: 'pause', label: 'Pause The Long Haul S02E03', group: 'Actions', icon: 'pause', keys: ['p'] },
    { id: 'replace', label: 'Replace with another release…', group: 'Actions', icon: 'swap', keys: ['R'] },
    { id: 'kill', label: 'Kill download…', group: 'Actions', icon: 'trash', danger: true, keys: ['delete'] },
    { id: 'go-dash', label: 'Dashboard', group: 'Screens', icon: 'home', keys: ['g', 'd'] },
    { id: 'go-queue', label: 'Queue', group: 'Screens', icon: 'activity', keys: ['g', 'q'], keywords: ['downloads', 'activity'] },
    { id: 'go-wanted', label: 'Wanted › Missing', group: 'Screens', icon: 'target', keys: ['g', 'w'] },
    { id: 'go-health', label: 'System › Health', group: 'Screens', icon: 'heart-pulse' },
    { id: 'tlh', label: 'The Long Haul', group: 'Items', kind: 'tv', subtitle: 'TV · 2024 · 3 seasons' },
    { id: 'dock', label: 'Dockside', group: 'Items', kind: 'movie', subtitle: 'Movie · 2026' },
    { id: 'lights', label: '港の灯り', group: 'Items', kind: 'music', subtitle: 'Album · Kaori Minato' },
    { id: 'idx', label: 'Harbour Tracker', group: 'Settings', icon: 'rss', subtitle: 'Settings › Indexers' },
    { id: 'qbit', label: 'qBittorrent', group: 'Settings', icon: 'download', subtitle: 'Settings › Download clients' },
  ];
  const { Story } = defineMeta({
    title: 'Organisms/CommandPalette',
    component: CommandPalette,
    tags: ['autodocs'],
    args: { items, recent: ['dock', 'go-queue'], query: '' },
  });
</script>

<script lang="ts">
  let open = $state(false);
  let picked = $state('');
</script>

<svelte:window onkeydown={(e) => { if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); open = true; } }} />

<Story name="Playground">
  {#snippet template(args)}<CommandPalette {...args} modal={false} open />{/snippet}
</Story>

<Story name="Modal (Ctrl/Cmd+K)" asChild>
  <div class="bar">
    <Button icon="command" onclick={() => (open = true)}>Open palette</Button>
    <span class="hint">or press <Kbd keys={['mod', 'k']} /></span>
    {#if picked}<span class="hint">Picked: {picked}</span>{/if}
  </div>
  <CommandPalette bind:open {items} recent={['dock', 'go-queue']} onselect={(i) => (picked = i.label)} />
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Empty query: recent first"><div class="w"><CommandPalette modal={false} open {items} recent={['dock', 'go-queue']} /></div></Row>
    <Row label="Fuzzy match with highlights"><div class="w"><CommandPalette modal={false} open {items} query="que" /></div></Row>
    <Row label="Searching items"><div class="w"><CommandPalette modal={false} open {items} query="long" loading /></div></Row>
    <Row label="No match"><div class="w"><CommandPalette modal={false} open {items} query="zzqx" /></div></Row>
  </ThemePair>
</Story>

<style>
  .bar { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-3); }
  .hint { font-size: var(--text-sm); color: var(--ink-muted); }
  .w { width: 100%; }
</style>
