<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Switch from './Switch.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Atoms/Switch', component: Switch, tags: ['autodocs'], args: { label: 'Enabled', checked: true } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Off, on, hidden label"><Switch label="RSS sync" /><Switch label="Automatic search" checked /><Switch label="Enable harbour-public" hideLabel checked /></Row>
    <Row label="Disabled"><Switch label="Managed by config" checked disabled /><Switch label="Off" disabled /></Row>
  </ThemePair>
</Story>
