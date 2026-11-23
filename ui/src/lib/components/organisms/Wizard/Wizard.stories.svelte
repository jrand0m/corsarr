<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Wizard, { type WizardStep } from './Wizard.svelte';
  import TextField from '../../molecules/TextField/TextField.svelte';
  import UrlField from '../../molecules/UrlField/UrlField.svelte';
  import EnumField from '../../molecules/EnumField/EnumField.svelte';
  import PropertyList from '../../molecules/PropertyList/PropertyList.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const steps: WizardStep[] = [
    { id: 'plugin', label: 'Choose plugin', description: 'Built-in plugins ship with corsarr; others load from the plugins folder.' },
    { id: 'connect', label: 'Connection', description: 'Where corsarr reaches the indexer and how it signs in.' },
    { id: 'caps', label: 'Categories', description: 'Map the indexer’s categories to your media kinds.', optional: true },
    { id: 'review', label: 'Review', description: 'Check the settings, then add the indexer.' },
  ];
  const plugins = [
    { value: 'torznab', label: 'Torznab feed', description: 'Any indexer that speaks Torznab.' },
    { value: 'newznab', label: 'Newznab feed', description: 'Usenet indexers.' },
  ];
  const { Story } = defineMeta({
    title: 'Organisms/Wizard',
    component: Wizard,
    tags: ['autodocs'],
    args: { steps, current: 'plugin', completed: [] },
  });
</script>

<script lang="ts">
  let name = $state('');
  let url = $state('');
  const validate = (s: string) =>
    s === 'connect'
      ? [!name && 'Give the indexer a name', !url && 'Enter the feed URL, e.g. https://feeds.example.net/api'].filter((x): x is string => !!x)
      : [];
</script>

{#snippet body(step: WizardStep)}
  {#if step.id === 'plugin'}
    <EnumField label="Plugin" appearance="radio" options={plugins} value="torznab" />
  {:else if step.id === 'connect'}
    <div class="form">
      <TextField label="Name" bind:value={name} placeholder="Harbour Tracker" />
      <UrlField label="Feed URL" bind:value={url} />
    </div>
  {:else if step.id === 'caps'}
    <p>Categories 5000–5999 map to TV; 2000–2999 map to Movies.</p>
  {:else}
    <PropertyList items={[
      { key: 'p', label: 'Plugin', value: 'Torznab feed' },
      { key: 'n', label: 'Name', value: name || 'Harbour Tracker' },
      { key: 'u', label: 'Feed URL', value: url || 'https://feeds.example.net/api', mono: true },
    ]} />
  {/if}
{/snippet}

<Story name="Playground">
  {#snippet template(args)}<Wizard {...args} children={body} {validate} oncancel={() => {}} finishLabel="Add indexer" />{/snippet}
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="First step"><div class="w"><Wizard {steps} children={body} oncancel={() => {}} /></div></Row>
    <Row label="Resumed at the last step (optional step skipped)">
      <div class="w"><Wizard {steps} current="review" completed={['plugin', 'connect']} resumed children={body} finishLabel="Add indexer" /></div>
    </Row>
    <Row label="Finishing (busy)">
      <div class="w"><Wizard {steps} current="review" completed={['plugin', 'connect', 'caps']} busy children={body} finishLabel="Add indexer" /></div>
    </Row>
  </ThemePair>
</Story>

<Story name="Validation errors (press Continue on Connection)" asChild>
  <div class="w"><Wizard {steps} current="connect" completed={['plugin']} children={body} {validate} oncancel={() => {}} finishLabel="Add indexer" /></div>
</Story>

<style>
  .w { width: 100%; }
  .form { display: grid; gap: var(--space-4); max-width: 28rem; }
</style>
