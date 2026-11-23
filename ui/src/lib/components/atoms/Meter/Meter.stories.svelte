<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Meter from './Meter.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({
    title: 'Atoms/Meter',
    component: Meter,
    tags: ['autodocs'],
    args: { label: 'Disk used', value: 0.71, warnAt: 0.85, errorAt: 0.95, showValue: true },
  });
</script>

<Story name="Playground" />

<Story name="All levels" asChild>
  <ThemePair>
    <div style="display: grid; gap: var(--space-3)">
      <Row label="Free space (high is good)"><div style="width: 100%"><Meter label="Free space on /srv/media" value={412} max={2000} better="high" warnAt={0.15} errorAt={0.05} valueText="412 GB free of 2 TB" showValue /></div></Row>
      <Row label="Nearly full"><div style="width: 100%"><Meter label="Free space on /srv/books" value={90} max={1000} better="high" warnAt={0.15} errorAt={0.05} valueText="90 GB free of 1 TB" showValue /></div></Row>
      <Row label="Out of space"><div style="width: 100%"><Meter label="Free space on /tmp" value={2} max={100} better="high" warnAt={0.15} errorAt={0.05} valueText="2 GB free of 100 GB" showValue /></div></Row>
      <Row label="Completion"><div style="width: 100%"><Meter label="Episodes on disk" value={38} max={42} valueText="38 of 42" showValue /></div></Row>
      <Row label="Quota used (low is good)"><div style="width: 100%"><Meter label="Daily API quota" value={0.9} warnAt={0.8} errorAt={0.95} showValue /></div></Row>
    </div>
  </ThemePair>
</Story>
