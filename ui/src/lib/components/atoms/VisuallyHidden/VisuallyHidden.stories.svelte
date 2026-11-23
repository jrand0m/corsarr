<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import VisuallyHidden from './VisuallyHidden.svelte';
  import Icon from '../Icon/Icon.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';

  const { Story } = defineMeta({ title: 'Atoms/VisuallyHidden', component: VisuallyHidden, tags: ['autodocs'] });
</script>

<Story name="Usage" asChild>
  <ThemePair>
    <p>
      <span style="color: var(--tone-error-ink)"><Icon name="x-circle" /></span>
      <VisuallyHidden>Failing: </VisuallyHidden>The text after the icon is read with a hidden prefix.
    </p>
  </ThemePair>
</Story>
