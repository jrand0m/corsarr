<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import DecisionPanel, { type Decision, type DecisionReason } from './DecisionPanel.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const now = Date.parse('2026-11-22T14:05:00Z');
  const accepted: Decision = {
    release: { title: 'Dockside.2026.1080p.WEB-DL.DDP5.1.H.264-HARBOUR', indexer: 'Harbour Tracker', protocol: 'torrent', size: 6.2e9, publishedAt: now - 7_200_000, seeders: 1240, quality: 'WEB-DL 1080p', runtimeSeconds: 6600 },
    verdict: 'accept', rank: 1, candidates: 14,
    score: { total: 1250, formats: [{ name: 'DD+ 5.1', score: 250 }, { name: 'Trusted group', score: 1000 }] },
    reasons: [],
  };
  const pref: DecisionReason = { rule: 'size.max', category: 'preference', message: '58.2 GB is over the 40 GB per-movie cap' };
  const temp: DecisionReason = { rule: 'seeders.min', category: 'temporary', message: '0 seeders, below the minimum of 1' };
  const perm: DecisionReason = { rule: 'blocklist', category: 'permanent', message: 'On the blocklist since 20 Nov (stalled twice)', blocklist: true };
  const floor: DecisionReason = { rule: 'quality.floor', category: 'permanent', message: 'HDTV 720p is below the profile floor of WEB-DL 1080p' };
  const safe: DecisionReason = { rule: 'payload.executable', category: 'safety', message: 'Archive contains an executable (setup.exe)' };
  const remux = { ...accepted.release, title: 'Dockside.2026.2160p.UHD.BluRay.REMUX.HDR.TrueHD.7.1-DOCKYARD', size: 58.2e9, seeders: 0, quality: 'Remux 2160p' };
  const prefOnly: Decision = { release: remux, verdict: 'reject', rank: 6, candidates: 14, score: { total: 1500, formats: [{ name: 'TrueHD Atmos', score: 500 }, { name: 'HDR10', score: 1000 }] }, reasons: [pref, temp] };
  const needsConfirm: Decision = { ...prefOnly, rank: 11, reasons: [pref, perm, floor] };
  const blocked: Decision = {
    release: { ...remux, title: 'Dockside.2026.1080p.WEB.x264-FREE.RAR', size: 1.1e9, protocol: 'usenet', quality: 'WEB 1080p' },
    verdict: 'reject', rank: 14, candidates: 14, score: { total: -500, formats: [{ name: 'Unknown group', score: -500 }] }, reasons: [safe, perm, pref],
  };

  const direct: Decision = {
    release: { title: 'Harbour Lights - Low Tide (2025) [FLAC]', indexer: 'TrubadurCamp', protocol: 'source', size: 312e6, publishedAt: now - 86_400_000, quality: 'FLAC', runtimeSeconds: 2640 },
    verdict: 'accept', rank: 1, candidates: 4, score: { total: 0, formats: [] }, reasons: [],
  };

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
    <Row label="Accepted, ranked first"><div class="w"><DecisionPanel decision={accepted} {now} ongrab={() => {}} /></div></Row>
    <Row label="Rejected, preference + temporary: grab anyway"><div class="w"><DecisionPanel decision={prefOnly} {now} ongrab={() => {}} /></div></Row>
    <Row label="Rejected, permanent + blocklist: grab anyway with confirm"><div class="w"><DecisionPanel decision={needsConfirm} {now} ongrab={() => {}} /></div></Row>
    <Row label="Rejected, safety: no override"><div class="w"><DecisionPanel decision={blocked} {now} ongrab={() => {}} /></div></Row>
    <Row label="Accepted, from a direct music source"><div class="w"><DecisionPanel decision={direct} {now} ongrab={() => {}} /></div></Row>
    <Row label="Read-only (no grab handler)"><div class="w"><DecisionPanel decision={accepted} {now} /></div></Row>
  </ThemePair>
</Story>

<style>
  .w { width: min(100%, 34rem); }
</style>
