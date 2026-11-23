<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Link from './Link.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';

  const { Story } = defineMeta({ title: 'Atoms/Link', component: Link, tags: ['autodocs'] });
</script>

<Story name="All variants" asChild>
  <ThemePair>
    <p>Rejected by the <Link href="#profile">Living-room TV</Link> profile.</p>
    <p><Link href="#item" variant="quiet">Harbour Lights · S02E05</Link> (quiet, for tables)</p>
    <p>Metadata from <Link href="https://example.org" external>the provider</Link>.</p>
  </ThemePair>
</Story>
