<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import PluginActionResult from './PluginActionResult.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({
    title: 'Molecules/PluginActionResult', component: PluginActionResult, tags: ['autodocs'],
    args: { state: 'ok', action: 'Test connection', text: 'Connected to Harbour 2.4 ✅' },
  });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Running"><PluginActionResult state="running" action="Test connection" /></Row>
    <Row label="OK"><PluginActionResult state="ok" action="Test connection" text="Connected to Harbour 2.4 ✅" /></Row>
    <Row label="Warning, two lines"><PluginActionResult state="warn" action="Find categories" text={'Found 3 categories.\nThe category "tv" is missing ⚠️'} /></Row>
    <Row label="Failed"><PluginActionResult state="fail" action="Test connection" text="The client refused the API key 🔑" /></Row>
    <Row label="Text refused"><PluginActionResult state="refused" action="Test connection" /></Row>
    <Row label="Hostile text stays literal"><PluginActionResult state="ok" text={'<script>alert(1)</script> <a href="javascript:alert(1)">open</a>'} /></Row>
  </ThemePair>
</Story>
