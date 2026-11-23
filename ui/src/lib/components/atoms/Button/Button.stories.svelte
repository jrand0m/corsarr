<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Button from './Button.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({
    title: 'Atoms/Button',
    component: Button,
    tags: ['autodocs'],
    args: { variant: 'secondary', size: 'md', loading: false, disabled: false },
    argTypes: {
      variant: { control: 'inline-radio', options: ['primary', 'secondary', 'ghost', 'danger'] },
      size: { control: 'inline-radio', options: ['sm', 'md', 'lg'] },
    },
  });
</script>

<Story name="Playground">
  {#snippet template(args)}<Button {...args}>Search now</Button>{/snippet}
</Story>

<Story name="All variants and states" asChild>
  <ThemePair>
    <Row label="Variants">
      <Button variant="primary" icon="search">Search selected</Button>
      <Button icon="pause">Pause</Button>
      <Button variant="ghost" icon="refresh">Refresh</Button>
      <Button variant="danger" icon="trash">Delete files</Button>
    </Row>
    <Row label="Sizes">
      <Button variant="primary" size="sm">Grab</Button>
      <Button variant="primary">Grab</Button>
      <Button variant="primary" size="lg">Grab</Button>
    </Row>
    <Row label="Disabled">
      <Button variant="primary" disabled>Import 12 files</Button>
      <Button disabled>Test</Button>
      <Button variant="ghost" disabled>Undo</Button>
    </Row>
    <Row label="Loading">
      <Button variant="primary" loading>Testing connection</Button>
      <Button loading>Saving</Button>
    </Row>
    <Row label="Trailing icon and link">
      <Button iconEnd="chevron-down">Act on 3</Button>
      <Button href="#health" variant="ghost" icon="heart-pulse">Open health</Button>
    </Row>
  </ThemePair>
</Story>
