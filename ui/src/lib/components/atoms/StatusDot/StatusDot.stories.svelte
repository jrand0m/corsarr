<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import StatusDot from './StatusDot.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Atoms/StatusDot', component: StatusDot, tags: ['autodocs'], args: { tone: 'warn', label: 'Integrations need attention', count: 2 } });
</script>

<Story name="Playground" />

<Story name="Tones and shapes" asChild>
  <ThemePair>
    <Row label="Shape also encodes tone: circle ok/info, diamond warn, square error">
      <StatusDot tone="ok" label="All integrations OK" showLabel />
      <StatusDot tone="warn" label="Degraded" count={2} showLabel />
      <StatusDot tone="error" label="Failing" count={1} showLabel />
      <StatusDot tone="info" label="Live" pulse showLabel />
      <StatusDot tone="neutral" label="Disconnected" showLabel />
    </Row>
    <Row label="Compact (label hidden)"><StatusDot tone="error" label="Integrations failing" count={3} /><StatusDot tone="ok" label="Connected" pulse /></Row>
  </ThemePair>
</Story>
