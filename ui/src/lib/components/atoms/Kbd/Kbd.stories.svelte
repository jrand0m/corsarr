<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Kbd from './Kbd.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Atoms/Kbd', component: Kbd, tags: ['autodocs'], args: { keys: ['mod', 'k'] } });
</script>

<Story name="Playground" />

<Story name="App shortcuts" asChild>
  <ThemePair>
    <Row label="Palette, go to, table keys">
      <Kbd keys={['mod', 'k']} />
      <Kbd keys={['g']} then={['d']} />
      <Kbd keys={['shift', 'x']} />
      <Kbd keys={['shift', 'up']} />
      <Kbd keys={['mod', 'enter']} />
      <Kbd keys={['?']} />
      <Kbd keys={['esc']} />
    </Row>
  </ThemePair>
</Story>
