<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import StatusBadge from './StatusBadge.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import { vocabulary, type StatusFamily } from '../../../status/vocabulary';

  const families = Object.keys(vocabulary) as StatusFamily[];
  const { Story } = defineMeta({
    title: 'Atoms/StatusBadge',
    component: StatusBadge,
    tags: ['autodocs'],
    args: { family: 'download', value: 'stalled', size: 'md', appearance: 'tint' },
    argTypes: { family: { control: 'select', options: families }, appearance: { control: 'inline-radio', options: ['tint', 'plain'] } },
  });
</script>

<Story name="Playground" />

<Story name="Full vocabulary" asChild>
  <ThemePair>
    {#each families as family (family)}
      <Row label={family}>
        {#each Object.keys(vocabulary[family]) as value (value)}<StatusBadge {family} {value} />{/each}
      </Row>
    {/each}
  </ThemePair>
</Story>

<Story name="Kind modes with tooltips" asChild>
  <ThemePair>
    <Row label="hover for the meaning">
      {#each Object.keys(vocabulary.kindMode) as value (value)}<StatusBadge family="kindMode" {value} />{/each}
    </Row>
  </ThemePair>
</Story>

<Story name="Sizes, plain, detail" asChild>
  <ThemePair>
    <Row label="sm / md"><StatusBadge family="health" value="failing" size="sm" /><StatusBadge family="health" value="failing" /></Row>
    <Row label="plain (dense table cells)"><StatusBadge family="download" value="downloading" appearance="plain" /><StatusBadge family="download" value="import-blocked" appearance="plain" /></Row>
    <Row label="with detail"><StatusBadge family="health" value="failing" detail="timeout" /><StatusBadge family="importOutcome" value="partial" detail="12 of 13" /></Row>
    <Row label="long label truncates"><span style="max-width: 9rem; display: inline-flex"><StatusBadge family="migration" value="managed" /></span></Row>
  </ThemePair>
</Story>
