<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import WizardPage from './WizardPage.svelte';
  import EnumField from '../../molecules/EnumField/EnumField.svelte';
  import Banner from '../../molecules/Banner/Banner.svelte';
  import SchemaForm from '../../organisms/SchemaForm/SchemaForm.svelte';
  import { torznabSchema, torznabValues, NOW } from '../../../storybook/fixtures';

  const steps = [
    { id: 'plugin', label: 'Choose plugin', description: 'Built-in plugins ship with corsarr; others load from the plugins folder.' },
    { id: 'connect', label: 'Connection', description: 'Where corsarr reaches the indexer and how it signs in.' },
    { id: 'caps', label: 'Categories', description: 'Map the indexer’s categories to your media kinds.', optional: true },
    { id: 'review', label: 'Review', description: 'Check the settings, then add the indexer.' },
  ];
  const plugins = [
    { value: 'torznab', label: 'Torznab feed', description: 'Any indexer that speaks Torznab.' },
    { value: 'newznab', label: 'Newznab feed', description: 'Usenet indexers.' },
  ];
  const back = { href: '#/settings/indexers', label: 'Indexers & sources' };
  const phone = { viewport: { value: 'phone', isRotated: false } };
  const desktop = { viewport: { value: 'desktop', isRotated: false } };

  const { Story } = defineMeta({ title: 'Templates/WizardPage', component: WizardPage, tags: ['autodocs'] });
</script>

{#snippet step(s: { id: string })}
  {#if s.id === 'plugin'}<EnumField label="Plugin" options={plugins} value="torznab" appearance="radio" />
  {:else if s.id === 'connect'}<SchemaForm schema={torznabSchema} values={torznabValues} label="Connection" now={NOW} />
  {:else}<p>Step “{s.id}”.</p>{/if}
{/snippet}
{#snippet help()}
  <h2 style="font-size: var(--text-md); color: var(--ink)">What happens next</h2>
  <p>corsarr tests the feed when you continue, reads its categories, and only saves once every check passes. Nothing is searched until you finish.</p>
{/snippet}

<Story name="Add indexer" asChild globals={desktop}>
  <WizardPage title="Add indexer" {back} {steps} current="connect" completed={['plugin']} children={step} aside={help} />
</Story>

<Story name="Resumed with banner" asChild globals={{ ...desktop, theme: 'dark' }}>
  <WizardPage title="Migration run" description="Bring indexers, clients and libraries over from your current setup." {steps} current="caps" completed={['plugin', 'connect']} resumed children={step}>
    {#snippet banner()}<Banner preset="preview" title="Dry run">Nothing changes until you apply the plan on the last step.</Banner>{/snippet}
  </WizardPage>
</Story>

<Story name="Phone" asChild globals={phone}>
  <WizardPage title="Add indexer" {back} {steps} current="plugin" children={step} aside={help} />
</Story>
