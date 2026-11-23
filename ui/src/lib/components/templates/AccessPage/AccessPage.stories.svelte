<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import AccessPage from './AccessPage.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import TextField from '../../molecules/TextField/TextField.svelte';

  const phone = { viewport: { value: 'phone', isRotated: false } };
  const desktop = { viewport: { value: 'desktop', isRotated: false } };
  const { Story } = defineMeta({ title: 'Templates/AccessPage', component: AccessPage, tags: ['autodocs'], parameters: { layout: 'fullscreen' } });
</script>

{#snippet form()}
  <form style="display: grid; gap: var(--space-4)" onsubmit={(e) => e.preventDefault()}>
    <TextField label="Password" />
    <Button type="submit" variant="primary">Sign in</Button>
  </form>
{/snippet}

<Story name="Narrow column" asChild globals={desktop}>
  <AccessPage title="Sign in" children={form} />
</Story>

<Story name="Wide column with footer, phone" asChild globals={{ ...phone, theme: 'dark' }}>
  <AccessPage title="Set up corsarr" description="Choose the admin password. You can change it later in Settings." width="wide" children={form}>
    {#snippet footer()}<p>Configuration comes from <code>/config/corsarr.yaml</code>.</p>{/snippet}
  </AccessPage>
</Story>
