<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import NumberInput from './NumberInput.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({
    title: 'Atoms/NumberInput',
    component: NumberInput,
    tags: ['autodocs'],
    args: { value: 30, min: 0, max: 1440, step: 5, unit: 'min', 'aria-label': 'Grace period' },
  });
</script>

<Story name="Playground" />

<Story name="All variants and states" asChild>
  <ThemePair>
    <Row label="Units"><NumberInput aria-label="Max size" value={12} unit="GB" step={0.5} /><NumberInput aria-label="Score" value={-50} step={10} /><NumberInput aria-label="Ratio" value={1.5} step={0.1} steppers={false} /></Row>
    <Row label="Sizes"><NumberInput aria-label="Small" size="sm" value={4} /><NumberInput aria-label="Large" size="lg" value={4} /></Row>
    <Row label="At limit, invalid"><NumberInput aria-label="At max" value={10} max={10} /><NumberInput aria-label="Invalid" value={0} invalid /></Row>
    <Row label="Managed, disabled"><NumberInput aria-label="Managed" value={6} unit="h" readonly /><NumberInput aria-label="Disabled" value={6} disabled /></Row>
  </ThemePair>
</Story>
