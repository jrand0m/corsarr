<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import QueueCard from './QueueCard.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import type { QueueCardData } from './QueueCard.svelte';
  const now = Date.parse('2026-11-22T14:05:00Z');
  const downloading: QueueCardData = {
    id: 'd1', title: 'The Long Haul', kind: 'tv', subtitle: 'S02E03 · Customs', state: 'downloading',
    progress: 0.46, size: 4.7e9, downloaded: 2.16e9, speed: 12.4e6, eta: 205, href: '#queue/qbit/d1',
  };
  const stalled: QueueCardData = {
    id: 'd2', title: 'Dockside', kind: 'movie', subtitle: '2026 · 2160p WEB-DL', state: 'stalled', reason: 'no peers for 2h',
    progress: 0.81, size: 18.2e9, downloaded: 14.7e9, speed: 0,
    rule: { badge: 'stalled', at: now + 245_000, action: 'Removes' },
  };
  const season: QueueCardData = {
    id: 'd3', title: 'The Long Haul', kind: 'tv', subtitle: 'S01 · 13 episodes', state: 'import-blocked', reason: 'sample only',
    size: 21e9, rule: { badge: 'stuck-import', at: now + 3 * 3600_000, action: 'Retries', dryRun: true },
  };
  const paused: QueueCardData = {
    id: 'd4', title: 'Harbour Lights', kind: 'music', subtitle: 'Album · 11 tracks · FLAC', state: 'queued', paused: true, held: true,
    progress: 0.1, size: 412e6, downloaded: 41e6,
  };
  const metadata: QueueCardData = {
    id: 'd5', title: 'A Very Long Book Title About Container Shipping and the Ports That Changed Everything', kind: 'book',
    subtitle: 'EPUB', state: 'downloading', shadow: true, reason: 'fetching metadata',
  };
  const done: QueueCardData = { id: 'd6', title: '港の灯り', kind: 'music', subtitle: 'Album · 9 tracks', state: 'imported', size: 98e6 };
  const { Story } = defineMeta({ title: 'Molecules/QueueCard', component: QueueCard, tags: ['autodocs'], args: { download: downloading, now } });
</script>

<Story name="Playground">
  {#snippet template(args)}<div class="w"><QueueCard {...args} onaction={() => {}} /></div>{/snippet}
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Downloading"><div class="w"><QueueCard download={downloading} {now} onaction={() => {}} /></div></Row>
    <Row label="Stalled, rule acts in 4:05"><div class="w"><QueueCard download={stalled} {now} onaction={() => {}} /></div></Row>
    <Row label="Import blocked, dry-run rule, artwork fallback"><div class="w"><QueueCard download={season} {now} art={null} onaction={() => {}} /></div></Row>
    <Row label="Paused and held"><div class="w"><QueueCard download={paused} {now} onaction={() => {}} /></div></Row>
    <Row label="Shadow, metadata pending, long title"><div class="w"><QueueCard download={metadata} {now} onaction={() => {}} /></div></Row>
    <Row label="Imported (no actions), CJK title"><div class="w"><QueueCard download={done} {now} onaction={() => {}} /></div></Row>
    <Row label="Read-only (no handler, as in a Telegram widget)"><div class="w"><QueueCard download={stalled} {now} /></div></Row>
    <Row label="Limited actions"><div class="w"><QueueCard download={downloading} {now} actions={['pause', 'kill']} onaction={() => {}} /></div></Row>
  </ThemePair>
</Story>

<style>
  .w { width: min(100%, 22rem); }
</style>
