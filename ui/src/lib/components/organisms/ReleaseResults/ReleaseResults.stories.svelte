<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ReleaseResults from './ReleaseResults.svelte';
  import type { Decision } from '../DecisionPanel/DecisionPanel.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';

  const now = Date.parse('2026-11-22T14:05:00Z');
  const rel = (title: string, size: number, seeders: number, leechers: number) => ({
    title, indexer: 'Harbour Tracker', protocol: 'torrent' as const, size, publishedAt: now - 7_200_000, seeders, quality: 'WEB-DL 1080p', runtimeSeconds: 6600,
    swarm: { seeders, leechers, reported: true, health: (seeders >= 20 ? 'good' : seeders >= 5 ? 'some' : seeders ? 'few' : 'none') as 'good' | 'some' | 'few' | 'none' },
    availability: { status: (seeders ? 'complete' : 'may_be_incomplete') as 'complete' | 'may_be_incomplete' },
  });
  const pick: Decision = { release: rel('Dockside.2026.1080p.WEB-DL.DDP5.1-HARBOUR', 6.2e9, 340, 12), verdict: 'accept', rank: 1, candidates: 2, score: { total: 250, formats: [] }, reasons: [],
    automation: { outcome: 'grab' }, desirability: { score: 100, position: 'on_target', qualitySteps: 0, cutoff: 'WEBDL-1080p', sizeDelta: 0.03, preferredSize: 6e9 } };
  const second: Decision = { release: rel('Dockside.2026.1080p.WEBRip.x264-QUAY', 4.1e9, 9, 60), verdict: 'accept', rank: 2, candidates: 2, score: { total: 0, formats: [] }, reasons: [],
    automation: { outcome: 'outranked', lostTo: 'lower custom-format score than the pick' }, desirability: { score: 74, position: 'below', qualitySteps: 0, cutoff: 'WEBDL-1080p', sizeDelta: -0.32, preferredSize: 6e9 } };
  const rejected: Decision = { release: rel('Dockside.2026.2160p.REMUX-DOCKYARD', 58e9, 0, 14), verdict: 'reject', score: { total: 0, formats: [] },
    reasons: [{ rule: 'size.max', category: 'permanent', message: 'size 58.0 GB exceeds max 9.5 GB', summary: 'Too large: 58.0 GB, max 9.5 GB' }],
    automation: { outcome: 'declined' }, desirability: null };
  const options = [{ value: 'automation', label: 'Automation order' }, { value: 'desirability', label: 'Closest to profile' }, { value: 'seeders', label: 'Most seeders' }];

  const { Story } = defineMeta({ title: 'Organisms/ReleaseResults', component: ReleaseResults, tags: ['autodocs'],
    args: { items: [pick, second, rejected].map((decision, i) => ({ key: String(i), decision })), sort: 'automation', sortOptions: options, now } });
</script>

<Story name="Playground">
  {#snippet template(args)}<div class="w"><ReleaseResults {...args} ongrab={() => {}} /></div>{/snippet}
</Story>

<Story name="Automation would grab nothing" asChild>
  <ThemePair>
    <div class="w"><ReleaseResults items={[{ key: 'r', decision: rejected }]} sort="desirability" sortOptions={options} {now} ongrab={() => {}}
      note="Automatic search would grab nothing: none of the 1 releases passed the profile. Most common reasons: size 58.0 GB exceeds max 9.5 GB (1). You can grab one anyway where the rules allow it." /></div>
  </ThemePair>
</Story>

<style>
  .w { width: min(100%, 36rem); }
</style>
