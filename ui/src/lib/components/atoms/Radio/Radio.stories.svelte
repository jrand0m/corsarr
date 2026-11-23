<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Radio from './Radio.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';

  const { Story } = defineMeta({ title: 'Atoms/Radio', component: Radio, tags: ['autodocs'] });
</script>

<script lang="ts">
  let mode = $state('remove-blocklist');
</script>

<Story name="Group" asChild>
  <ThemePair>
    <fieldset style="border: 0; padding: 0; display: grid; gap: var(--space-2)">
      <legend style="font-weight: var(--weight-medium); margin-bottom: var(--space-2)">When killing a download</legend>
      <Radio bind:group={mode} value="remove-blocklist" label="Remove, blocklist and search again" />
      <Radio bind:group={mode} value="remove" label="Remove only" description="The release can be grabbed again." />
      <Radio bind:group={mode} value="never" label="Keep files and do nothing" disabled />
    </fieldset>
  </ThemePair>
</Story>
