<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import DecisionPanel, { type Decision, type DecisionReason } from './DecisionPanel.svelte';
  import DecisionPanelBefore from './before/DecisionPanelBefore.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const now = Date.parse('2026-11-22T14:05:00Z');
  const swarm = (seeders: number, leechers: number) => ({ seeders, leechers, reported: true, health: (seeders === 0 ? 'none' : seeders < 5 ? 'few' : seeders < 20 ? 'some' : 'good') as 'none' | 'few' | 'some' | 'good' });
  const accepted: Decision = {
    release: { title: 'Dockside.2026.1080p.WEB-DL.DDP5.1.H.264-HARBOUR', indexer: 'Harbour Tracker', protocol: 'torrent', size: 6.2e9, publishedAt: now - 7_200_000, seeders: 1240, quality: 'WEB-DL 1080p', runtimeSeconds: 6600,
      swarm: swarm(1240, 86), availability: { status: 'complete', grabs: 4120, files: 1 } },
    verdict: 'accept', rank: 1, candidates: 14,
    score: { total: 1250, formats: [{ name: 'DD+ 5.1', score: 250 }, { name: 'Trusted group', score: 1000 }] },
    reasons: [],
    automation: { outcome: 'grab' },
    desirability: { score: 100, position: 'on_target', qualitySteps: 0, cutoff: 'WEBDL-1080p', sizeDelta: 0.03, preferredSize: 6.0e9 },
  };
  const outranked: Decision = {
    ...accepted,
    release: { ...accepted.release, title: 'Dockside.2026.1080p.BluRay.x264-QUAY', size: 11.8e9, seeders: 3, quality: 'Bluray-1080p', swarm: swarm(3, 41), availability: { status: 'complete', grabs: 96 } },
    rank: 2, score: { total: 0, formats: [] },
    automation: { outcome: 'outranked', lostTo: 'lower custom-format score than the pick' },
    desirability: { score: 47, position: 'above', qualitySteps: 1, cutoff: 'WEBDL-1080p', sizeDelta: 0.97, preferredSize: 6.0e9 },
  };
  const pref: DecisionReason = { rule: 'size.max', category: 'preference', message: 'size 58.2 GB exceeds max 40.0 GB for Remux-2160p at 110 min', summary: 'Too large: 58.2 GB, max 40.0 GB' };
  const temp: DecisionReason = { rule: 'seeders.min', category: 'temporary', message: '0 seeders, need at least 1', summary: 'Too few seeders: 0, needs 1' };
  const perm: DecisionReason = { rule: 'blocklist', category: 'permanent', message: 'release is blocklisted for this item', summary: 'Blocklisted', blocklist: true };
  const floor: DecisionReason = { rule: 'quality.not_allowed', category: 'permanent', message: 'quality Remux-2160p is not allowed by profile "HD-1080p"', summary: 'Remux-2160p is not in the profile' };
  const safe: DecisionReason = { rule: 'payload.dangerous', category: 'safety', message: 'executable or script in payload: setup.exe', summary: 'Unsafe file inside: setup.exe' };
  const remux = { ...accepted.release, title: 'Dockside.2026.2160p.UHD.BluRay.REMUX.HDR.TrueHD.7.1-DOCKYARD', size: 58.2e9, seeders: 0, quality: 'Remux 2160p', swarm: swarm(0, 14), availability: { status: 'may_be_incomplete' as const, grabs: 12 } };
  const prefOnly: Decision = { release: remux, verdict: 'reject', rank: 6, candidates: 14, score: { total: 1500, formats: [{ name: 'TrueHD Atmos', score: 500 }, { name: 'HDR10', score: 1000 }] }, reasons: [pref, temp], automation: { outcome: 'declined' }, desirability: { score: 30, position: 'above', qualitySteps: 2, cutoff: 'WEBDL-1080p', sizeDelta: 8.7, preferredSize: 6.0e9 } };
  const needsConfirm: Decision = { ...prefOnly, rank: 11, reasons: [pref, perm, floor], desirability: null };
  const blocked: Decision = {
    release: { ...remux, title: 'Dockside.2026.1080p.WEB.x264-FREE.RAR', size: 1.1e9, protocol: 'usenet', quality: 'WEB 1080p', swarm: undefined, availability: { status: 'unknown', grabs: 57 } },
    verdict: 'reject', rank: 14, candidates: 14, score: { total: -500, formats: [{ name: 'Unknown group', score: -500 }] }, reasons: [safe, perm, pref], automation: { outcome: 'declined' },
    desirability: { score: 30, position: 'below', qualitySteps: 0, cutoff: 'WEBDL-1080p', sizeDelta: -0.82, preferredSize: 6.0e9, formatShortfall: 500, formatTarget: 0 },
  };
  const direct: Decision = {
    release: { title: 'Harbour Lights - Low Tide (2025) [FLAC]', indexer: 'TrubadurCamp', protocol: 'source', size: 312e6, publishedAt: now - 86_400_000, quality: 'FLAC', runtimeSeconds: 2640, availability: { status: 'complete', files: 11 } },
    verdict: 'accept', rank: 1, candidates: 4, score: { total: 0, formats: [] }, reasons: [], automation: { outcome: 'shadow' },
    desirability: { score: 100, position: 'on_target', qualitySteps: 0, cutoff: 'FLAC', sizeDelta: -0.04, preferredSize: 325e6 },
  };
  const pending: Decision = { ...outranked, automation: { outcome: 'pending' } };
  const results = [accepted, outranked, prefOnly, needsConfirm, blocked];

  const { Story } = defineMeta({
    title: 'Organisms/DecisionPanel',
    component: DecisionPanel,
    tags: ['autodocs'],
    args: { decision: needsConfirm, now },
  });
</script>

<Story name="Playground">
  {#snippet template(args)}<div class="w"><DecisionPanel {...args} ongrab={() => {}} /></div>{/snippet}
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Automation pick"><div class="w"><DecisionPanel decision={accepted} {now} ongrab={() => {}} /></div></Row>
    <Row label="Automation pick, shadow mode (direct music source)"><div class="w"><DecisionPanel decision={direct} {now} ongrab={() => {}} /></div></Row>
    <Row label="Accepted, outranked by the pick"><div class="w"><DecisionPanel decision={outranked} {now} ongrab={() => {}} /></div></Row>
    <Row label="Deciding (indexers still answering)"><div class="w"><DecisionPanel decision={pending} {now} ongrab={() => {}} /></div></Row>
    <Row label="Rejected, preference + temporary: grab anyway"><div class="w"><DecisionPanel decision={prefOnly} {now} ongrab={() => {}} /></div></Row>
    <Row label="Rejected, permanent + blocklist, outside the profile: grab anyway with confirm"><div class="w"><DecisionPanel decision={needsConfirm} {now} ongrab={() => {}} /></div></Row>
    <Row label="Rejected, safety, usenet: no override"><div class="w"><DecisionPanel decision={blocked} {now} ongrab={() => {}} /></div></Row>
    <Row label="Read-only (no grab handler)"><div class="w"><DecisionPanel decision={accepted} {now} /></div></Row>
  </ThemePair>
</Story>

<Story name="Rejection collapsed" asChild>
  <ThemePair><div class="w"><DecisionPanel decision={needsConfirm} {now} ongrab={() => {}} /></div></ThemePair>
</Story>

<Story name="Rejection expanded" asChild>
  <ThemePair><div class="w"><DecisionPanel decision={needsConfirm} {now} expanded ongrab={() => {}} /></div></ThemePair>
</Story>

<Story name="Before and after" asChild>
  <div class="ba">
    <section>
      <h3>Before</h3>
      <ol>{#each results as d, i (i)}<li><DecisionPanelBefore decision={d} {now} ongrab={() => {}} /></li>{/each}</ol>
    </section>
    <section>
      <h3>After</h3>
      <ol>{#each results as d, i (i)}<li><DecisionPanel decision={d} {now} ongrab={() => {}} /></li>{/each}</ol>
    </section>
  </div>
</Story>

<style>
  .w { width: min(100%, 34rem); }
  .ba { display: grid; grid-template-columns: repeat(auto-fit, minmax(22rem, 1fr)); gap: var(--space-6); padding: var(--space-4); background: var(--surface); }
  .ba ol { display: grid; gap: var(--space-3); margin: 0; padding: 0; list-style: none; }
  .ba h3 { margin-bottom: var(--space-3); }
</style>
