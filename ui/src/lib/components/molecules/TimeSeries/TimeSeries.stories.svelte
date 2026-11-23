<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import TimeSeries from './TimeSeries.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import type { TimeBucket } from './TimeSeries.svelte';
  const end = Date.parse('2026-11-22T14:00:00Z');
  // Deterministic synthetic load: a daily rhythm plus a short outage.
  const day: TimeBucket[] = Array.from({ length: 24 }, (_, i) => {
    const v = Math.round(18 + 14 * Math.sin((i / 24) * Math.PI * 2 - 1.2) + ((i * 7) % 5));
    return { at: end - (23 - i) * 3600_000, value: i === 15 ? 2 : v, failed: i === 15 ? 11 : i === 16 ? 3 : i % 9 === 0 ? 1 : 0 };
  });
  const week: TimeBucket[] = Array.from({ length: 7 }, (_, i) => ({ at: end - (6 - i) * 86400_000, value: 380 + ((i * 53) % 120), failed: i === 4 ? 40 : 0 }));
  const clean: TimeBucket[] = day.map((b) => ({ ...b, failed: 0 }));
  const runs: TimeBucket[] = Array.from({ length: 30 }, (_, i) => ({ at: end - (29 - i) * 86400_000, value: i === 22 || i === 23 ? 0 : 1, failed: i === 22 || i === 23 ? 1 : 0 }));
  const { Story } = defineMeta({ title: 'Molecules/TimeSeries', component: TimeSeries, tags: ['autodocs'], args: { title: 'Queries per hour', buckets: day, range: '24h' } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="24 h with failures"><TimeSeries title="Queries per hour" buckets={day} range="24h" /></Row>
    <Row label="7 d"><TimeSeries title="Grabs per day" unit="grabs" buckets={week} range="7d" /></Row>
    <Row label="No failures (single series, no legend)"><TimeSeries title="Queries per hour" buckets={clean} range="24h" ranges={['24h']} /></Row>
    <Row label="Self-test strip"><TimeSeries title="Media server self-test" unit="runs" mode="strip" buckets={runs} range="30d" ranges={['30d']} /></Row>
    <Row label="Empty"><TimeSeries title="Queries per hour" buckets={[]} range="24h" /></Row>
    <Row label="Loading"><TimeSeries title="Queries per hour" buckets={[]} loading /></Row>
  </ThemePair>
</Story>
