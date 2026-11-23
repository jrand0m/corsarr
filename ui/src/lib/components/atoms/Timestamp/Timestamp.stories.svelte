<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Timestamp from './Timestamp.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const now = new Date('2026-11-22T14:05:00');
  const ago = (s: number) => new Date(now.getTime() - s * 1000);
  const { Story } = defineMeta({ title: 'Atoms/Timestamp', component: Timestamp, tags: ['autodocs'], args: { value: ago(300), now } });
</script>

<Story name="Playground" />

<Story name="Past, future, absolute" asChild>
  <ThemePair>
    <Row label="Past"><Timestamp value={ago(20)} {now} /><Timestamp value={ago(300)} {now} /><Timestamp value={ago(7200)} {now} /><Timestamp value={ago(86400)} {now} /><Timestamp value={ago(86400 * 40)} {now} /></Row>
    <Row label="Future"><Timestamp value={ago(-600)} {now} /><Timestamp value={ago(-3 * 3600)} {now} /><Timestamp value={ago(-86400 * 2)} {now} /></Row>
    <Row label="Absolute"><Timestamp value={now} mode="absolute" /></Row>
    <Row label="In a table row (not focusable)"><Timestamp value={ago(900)} {now} focusable={false} /></Row>
  </ThemePair>
</Story>
