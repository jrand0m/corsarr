<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Progress from './Progress.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({
    title: 'Atoms/Progress',
    component: Progress,
    tags: ['autodocs'],
    args: { label: 'Download progress', value: 0.62, showValue: true, tone: 'info', size: 'md' },
  });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Determinate, by tone">
      <div style="display: grid; gap: var(--space-3); width: 100%">
        <Progress label="Downloading" value={0.62} showValue />
        <Progress label="Stalled" value={0.31} tone="warn" showValue />
        <Progress label="Failed" value={0.88} tone="error" showValue />
        <Progress label="Completed" value={1} tone="ok" showValue />
      </div>
    </Row>
    <Row label="Partial (12 of 13)"><div style="width: 100%"><Progress label="Imported files" done={12} total={13} tone="warn" showValue /></div></Row>
    <Row label="Indeterminate (striped and still under reduced motion)"><div style="width: 100%"><Progress label="Checking files" /></div></Row>
    <Row label="Small"><div style="width: 100%"><Progress label="Small" size="sm" value={0.4} /></div></Row>
  </ThemePair>
</Story>
