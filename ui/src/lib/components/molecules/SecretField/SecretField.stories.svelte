<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SecretField from './SecretField.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Molecules/SecretField', component: SecretField, tags: ['autodocs'], args: { label: 'API key', isSet: true, help: 'Find it on the indexer under Profile › API.' } });
</script>

<Story name="Playground" />

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Not set"><SecretField label="API key" required help="Find it on the indexer under Profile › API." /></Row>
    <Row label="Set (stored, never shown)"><SecretField label="API key" isSet /></Row>
    <Row label="Typing a replacement"><SecretField label="Password" isSet value="hunter2-but-longer" /></Row>
    <Row label="From an environment variable"><SecretField label="API key" ref={'${env:NZBGEEK_API_KEY}'} /></Row>
    <Row label="From a file"><SecretField label="Password" ref={'${file:/run/secrets/qbit_password}'} /></Row>
    <Row label="Error"><SecretField label="API key" value="abc" error="The indexer rejected this key (HTTP 401)." /></Row>
    <Row label="Managed by config"><SecretField label="API key" ref={'${env:NZBGEEK_API_KEY}'} managed="indexers.nzbgeek.api_key" /></Row>
    <Row label="Drift"><SecretField label="API key" isSet drift="The config file's value differs from the stored one." /></Row>
    <Row label="Disabled"><SecretField label="API key" disabled /></Row>
  </ThemePair>
</Story>
