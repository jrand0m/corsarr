<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PlanDiff, { type PlanGroup } from './PlanDiff.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const pending: PlanGroup[] = [
    {
      type: 'Indexers',
      ops: [
        { id: 'i1', op: 'create', name: 'Harbour Tracker', fields: [
          { path: 'url', after: 'https://feeds.example.net/api' },
          { path: 'categories.tv', after: '5000, 5030, 5040' },
          { path: 'api_key', secret: true },
        ] },
        { id: 'i2', op: 'update', name: 'Dock Feeds', fields: [
          { path: 'priority', before: '25', after: '10' },
          { path: 'enabled', before: 'false', after: 'true' },
          { path: 'api_key', secret: true },
        ] },
      ],
    },
    {
      type: 'Quality profiles',
      ops: [
        { id: 'q1', op: 'update', name: 'HD 1080p', fields: [{ path: 'cutoff', before: 'WEB-DL 720p', after: 'WEB-DL 1080p' }] },
        { id: 'q2', op: 'prune', name: 'SD only' },
        { id: 'q3', op: 'skip', name: 'Ultra HD', reason: 'Last profile for Movies; at least one is kept.', inUseBy: [{ label: '214 movies', href: '#/library/movies?profile=uhd' }] },
        { id: 'q4', op: 'deferred', name: 'Anime 1080p', reason: 'Pruning waits for 3 downloads to finish importing.' },
      ],
    },
  ];
  const drift: PlanGroup[] = [
    { type: 'Download clients', ops: [{ id: 'd1', op: 'update', name: 'qBittorrent', fields: [{ path: 'password', secret: true }, { path: 'category', before: 'tv', after: 'tv-corsarr' }] }] },
  ];

  const { Story } = defineMeta({
    title: 'Organisms/PlanDiff',
    component: PlanDiff,
    tags: ['autodocs'],
    args: { groups: pending },
  });
</script>

<Story name="Playground">
  {#snippet template(args)}<div class="w"><PlanDiff {...args} onconfirm={() => {}} oncancel={() => {}} /></div>{/snippet}
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Pending plan: include toggles, Apply / Cancel"><div class="w"><PlanDiff groups={pending} onconfirm={() => {}} oncancel={() => {}} /></div></Row>
    <Row label="Pending, one operation left out"><div class="w"><PlanDiff groups={pending} excluded={['q2']} onconfirm={() => {}} oncancel={() => {}} /></div></Row>
    <Row label="Applying"><div class="w"><PlanDiff groups={drift} busy onconfirm={() => {}} oncancel={() => {}} /></div></Row>
    <Row label="Drift (read-only), secret differs"><div class="w"><PlanDiff groups={drift} label="Drift" /></div></Row>
    <Row label="Nothing to change"><div class="w"><PlanDiff groups={[]} /></div></Row>
  </ThemePair>
</Story>

<style>
  .w { width: min(100%, 40rem); }
</style>
