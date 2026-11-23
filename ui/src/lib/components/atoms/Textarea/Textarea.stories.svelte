<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Textarea from './Textarea.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Atoms/Textarea', component: Textarea, tags: ['autodocs'], args: { 'aria-label': 'Notes', rows: 3 } });
  const yaml = 'indexers:\n  - name: harbour-public\n    priority: 25';
</script>

<Story name="Playground" />

<Story name="All variants and states" asChild>
  <ThemePair>
    <Row label="Mono (YAML paste)"><Textarea aria-label="YAML" mono value={yaml} /></Row>
    <Row label="Invalid"><Textarea aria-label="Regex" mono invalid value="(?<group" rows={2} /></Row>
    <Row label="Managed, disabled"><Textarea aria-label="Managed" readonly value="Set in corsarr.yaml" rows={2} /><Textarea aria-label="Disabled" disabled value="Off" rows={2} /></Row>
  </ThemePair>
</Story>
