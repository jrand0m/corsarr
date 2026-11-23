<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Spinner from './Spinner.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Atoms/Spinner', component: Spinner, tags: ['autodocs'] });
</script>

<Story name="Sizes" asChild>
  <ThemePair>
    <Row label="Inline busy mark (decorative, or labelled)">
      <Spinner size="sm" /><Spinner /><Spinner size="lg" label="Testing connection" />
      <span style="color: var(--accent-ink)"><Spinner /></span>
    </Row>
  </ThemePair>
</Story>
