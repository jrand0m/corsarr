<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Timeline, { type TimelineEntry } from './Timeline.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const now = new Date(2026, 10, 22, 14, 5);
  const t = (d: number, h: number, m = 0) => new Date(2026, 10, d, h, m);
  const download: TimelineEntry[] = [
    { id: '1', kind: 'apply', at: t(21, 21, 4), title: 'Grab sent to Harbour box', status: { family: 'delivery', value: 'delivered' }, actor: 'Search: The Long Haul S02E03', fields: [{ label: 'Release', value: 'The.Long.Haul.S02E03.1080p.WEB-DL-HARBOUR', mono: true }, { label: 'Indexer', value: 'nzbgeek' }] },
    { id: '2', kind: 'transition', at: t(21, 21, 5), title: 'Download started', from: { family: 'download', value: 'queued' }, to: { family: 'download', value: 'downloading' } },
    { id: '3', kind: 'observation', at: t(21, 23, 40), title: 'No progress for 30 minutes', detail: 'Speed 0 B/s, 3 peers, 61% done', actor: 'Watcher' },
    { id: '4', kind: 'transition', at: t(21, 23, 40), title: 'Marked stalled', from: { family: 'download', value: 'downloading' }, to: { family: 'download', value: 'stalled' }, href: '#why', hrefLabel: 'Why stalled?' },
    { id: '5', kind: 'attempt', at: t(22, 0, 10), title: 'Retry 1 of 3: re-announce', status: { family: 'job', value: 'failed' }, detail: 'The tracker answered 503.', actor: 'Rule: stalled downloads' },
    { id: '6', kind: 'attempt', at: t(22, 9, 10), title: 'Retry 2 of 3: re-announce', status: { family: 'job', value: 'done' }, actor: 'Rule: stalled downloads' },
    { id: '7', kind: 'transition', at: t(22, 9, 12), title: 'Download resumed', from: { family: 'download', value: 'stalled' }, to: { family: 'download', value: 'downloading' } },
    { id: '8', kind: 'transition', at: t(22, 13, 50), title: 'Imported to the library', from: { family: 'download', value: 'importing' }, to: { family: 'download', value: 'imported' }, detail: '/tv/The Long Haul/Season 02/S02E03.mkv' },
    { id: '9', kind: 'note', at: t(22, 13, 58), title: 'Pinned to this release', actor: 'You' },
  ];

  const { Story } = defineMeta({ title: 'Organisms/Timeline', component: Timeline, tags: ['autodocs'], args: { entries: download, label: 'History of The Long Haul S02E03', now } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Newest first, grouped by day"><div style="width: 100%"><Timeline entries={download} label="History" {now} /></div></Row>
    <Row label="Oldest first, collapsed after 4"><div style="width: 100%"><Timeline entries={download} label="History, collapsed" order="oldest" limit={4} {now} /></div></Row>
    <Row label="Ungrouped"><div style="width: 100%"><Timeline entries={download.slice(0, 3)} label="History, ungrouped" groupByDay={false} {now} /></div></Row>
    <Row label="Loading"><div style="width: 100%"><Timeline entries={[]} label="History, loading" loading /></div></Row>
    <Row label="Empty"><div style="width: 100%"><Timeline entries={[]} label="History, empty" emptyTitle="No attempts yet" /></div></Row>
  </ThemePair>
</Story>
