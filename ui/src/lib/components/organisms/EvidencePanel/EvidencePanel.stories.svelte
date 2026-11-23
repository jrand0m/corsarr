<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import EvidencePanel, { type Evidence } from './EvidencePanel.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const now = Date.parse('2026-11-22T14:05:00Z');
  const stalled: Evidence = {
    rule: { id: 'stalled', name: 'Stalled downloads', href: '#/activity/automation/rules/stalled' },
    mode: 'live',
    classification: 'would-act',
    verdict: 'No progress for 2h 14m with 0 peers, over the 2h limit.',
    thresholds: [
      { label: 'No progress for', limit: '2h', observed: '2h 14m', exceeded: true },
      { label: 'Peers', limit: '1', observed: '0', exceeded: true },
      { label: 'Age', limit: '30m', observed: '4h 12m', exceeded: false },
    ],
    observations: [
      { at: now - 15_120_000, text: 'Grabbed from Harbour Tracker' },
      { at: now - 8_040_000, text: 'Last progress, at 81%' },
      { at: now - 600_000, text: 'Tracker reported 0 peers' },
    ],
    action: { summary: 'Removes the download, blocklists the release and searches again', verb: 'Removes', at: now + 245_000 },
    links: [{ label: 'Open download', href: '#/activity/queue/qbit/d2' }, { label: 'Rule settings', href: '#/settings/automation/stalled' }],
  };
  const dryRun: Evidence = {
    ...stalled,
    rule: { id: 'stuck-import', name: 'Stuck imports' },
    mode: 'dry-run',
    verdict: 'Import has waited 3h 40m for a manual decision, over the 3h limit.',
    thresholds: [{ label: 'Waiting for', limit: '3h', observed: '3h 40m', exceeded: true }],
    observations: [{ at: now - 13_200_000, text: 'Import blocked: sample only' }],
    action: { summary: 'Retries the import with the next file', verb: 'Retries', at: now + 3 * 3600_000 },
    links: [],
  };
  const acted: Evidence = {
    ...stalled,
    classification: 'acted',
    verdict: 'Removed after 2h 14m without progress and 0 peers.',
    undoUntil: now + 30 * 86_400_000,
  };
  const rejection: Evidence = {
    rule: { id: 'decision', name: 'Release decision' },
    mode: 'live',
    verdict: 'Rejected Dockside.2026.2160p.REMUX: three rules failed.',
    reasons: [
      { category: 'safety', message: 'Archive contains an executable (setup.exe)' },
      { category: 'permanent', message: 'Release is on the blocklist since 20 Nov' },
      { category: 'preference', message: '58.2 GB is over the 40 GB per-movie cap' },
    ],
  };
  const grace: Evidence = {
    rule: { id: 'metadata-timeout', name: 'Metadata timeout' },
    mode: 'live',
    classification: 'grace',
    verdict: 'Still fetching metadata after 4m; the limit is 10m.',
    thresholds: [{ label: 'Fetching metadata for', limit: '10m', observed: '4m', exceeded: false }],
  };

  const { Story } = defineMeta({
    title: 'Organisms/EvidencePanel',
    component: EvidencePanel,
    tags: ['autodocs'],
    args: { evidence: stalled, now, variant: 'panel' },
  });
</script>

<Story name="Playground">
  {#snippet template(args)}<div class="w"><EvidencePanel {...args} onundo={() => {}} /></div>{/snippet}
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Live rule, countdown to action"><div class="w"><EvidencePanel evidence={stalled} {now} /></div></Row>
    <Row label="Dry run (would act)"><div class="w"><EvidencePanel evidence={dryRun} {now} /></div></Row>
    <Row label="Acted, undo available"><div class="w"><EvidencePanel evidence={acted} {now} onundo={() => {}} /></div></Row>
    <Row label="Rejection reasons by category"><div class="w"><EvidencePanel evidence={rejection} {now} /></div></Row>
    <Row label="In grace, within limits"><div class="w"><EvidencePanel evidence={grace} {now} /></div></Row>
    <Row label="Popover variant (Why?)"><EvidencePanel evidence={stalled} {now} variant="popover" /></Row>
  </ThemePair>
</Story>

<style>
  .w { width: min(100%, 28rem); }
</style>
