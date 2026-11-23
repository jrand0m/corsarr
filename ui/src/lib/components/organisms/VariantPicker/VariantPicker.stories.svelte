<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import VariantPicker, { type Variant } from "./VariantPicker.svelte";
  import Button from "../../atoms/Button/Button.svelte";
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const now = Date.parse('2026-11-22T14:05:00Z');
  const variants: Variant[] = [
    { id: 'a', title: 'The.Long.Haul.S02E03.1080p.WEB.h264-HARBOUR', quality: 'WEB 1080p', size: 2.4e9, score: 1250, reason: 'Same group as the season, seeded 412', indexer: 'Harbour Tracker' },
    { id: 'b', title: 'The.Long.Haul.S02E03.1080p.AMZN.WEB-DL.DDP5.1.H.264-QUAY', quality: 'WEB-DL 1080p', size: 3.1e9, score: 1100, reason: 'Better audio, seeded 88', indexer: 'Dock Feeds' },
    { id: 'c', title: 'The.Long.Haul.S02E03.720p.HDTV.x264-CRANE', quality: 'HDTV 720p', size: 0.9e9, score: -200, reason: 'Below cutoff; would upgrade later', indexer: 'Usenet Pier' },
  ];
  const lower: Variant[] = [
    { id: 'l1', title: 'Dockside.2026.1080p.WEB-DL.DDP5.1-HARBOUR', quality: 'WEB-DL 1080p', size: 6.2e9, score: 900, reason: '12.0 GB smaller than the 2160p download' },
    { id: 'l2', title: 'Dockside.2026.1080p.BluRay.x264-QUAY', quality: 'Bluray 1080p', size: 9.8e9, score: 750, reason: 'Well seeded (1,240)' },
  ];

  const { Story } = defineMeta({
    title: 'Organisms/VariantPicker',
    component: VariantPicker,
    tags: ['autodocs'],
    args: { variants, defaultId: 'a', autoAcceptAt: now + 42_000, now },
  });
</script>

<script lang="ts">
  let result = $state('');
  let liveAt = $state(Date.now() + 30_000);
</script>

<Story name="Playground">
  {#snippet template(args)}<VariantPicker {...args} onpick={() => {}} oncancel={() => {}} />{/snippet}
</Story>

<Story name="Live countdown (auto-picks in 30 s)" asChild>
  <div class="stack">
    {#key liveAt}<VariantPicker {variants} defaultId="a" autoAcceptAt={liveAt} onpick={(id, auto) => (result = `${auto ? 'Auto-picked' : 'Picked'} ${id}`)} oncancel={() => (result = 'Cancelled')} />{/key}
    <p class="out" role="status">{result}</p>
    <Button size="sm" icon="refresh" onclick={() => { result = ''; liveAt = Date.now() + 30_000; }}>Restart</Button>
  </div>
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Default with countdown to auto-pick"><VariantPicker {variants} defaultId="a" autoAcceptAt={now + 42_000} {now} onpick={() => {}} oncancel={() => {}} /></Row>
    <Row label="Lower resolution (capped)"><VariantPicker variants={lower} cap="Below 2160p" title="Replace with lower resolution" {now} onpick={() => {}} oncancel={() => {}} /></Row>
    <Row label="No auto-pick"><VariantPicker {variants} defaultId="b" {now} onpick={() => {}} oncancel={() => {}} /></Row>
    <Row label="No alternatives"><VariantPicker variants={[]} cap="Below 1080p" onsearch={() => {}} /></Row>
    <Row label="Telegram widget (script-free form)"><VariantPicker {variants} defaultId="a" autoAcceptAt={now + 42_000} {now} formAction="/telegram/replace/d2" /></Row>
  </ThemePair>
</Story>

<style>
  .stack { display: grid; gap: var(--space-2); justify-items: start; }
  .out { font-size: var(--text-sm); color: var(--ink-muted); min-height: 1lh; }
</style>
