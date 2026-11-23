<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SettingsPage from './SettingsPage.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import CheckResults from '../../molecules/CheckResults/CheckResults.svelte';
  import EmptyState from '../../molecules/EmptyState/EmptyState.svelte';
  import { settingsSections, torznabSchema, torznabValues, torznabMeta, torznabManagedMeta, NOW } from '../../../storybook/fixtures';

  const checks = [
    { id: 'reach', name: 'Feed answers', status: 'pass' as const, ms: 412 },
    { id: 'auth', name: 'API key accepted', status: 'pass' as const, ms: 380 },
    { id: 'caps', name: 'Categories read', status: 'warn' as const, detail: 'Books (7000) is not offered by this feed.' },
  ];
  const phone = { viewport: { value: 'phone', isRotated: false } };
  const desktop = { viewport: { value: 'desktop', isRotated: false } };

  const { Story } = defineMeta({ title: 'Templates/SettingsPage', component: SettingsPage, tags: ['autodocs'] });
</script>

{#snippet testSlot()}<CheckResults title="Test on save" {checks} ranAt={NOW - 4000} now={new Date(NOW)} onrerun={() => {}} />{/snippet}
{#snippet testAction()}<Button icon="bolt">Test</Button>{/snippet}

<Story name="Object editor" asChild globals={desktop}>
  <SettingsPage sections={settingsSections} current="indexers" title="Harbour Tracker" description="Torznab feed · added 12 Nov"
    actions={testAction} schema={torznabSchema} values={torznabValues} meta={torznabMeta} test={testSlot} onsave={() => {}} oncancel={() => {}} now={NOW} />
</Story>

<Story name="Managed by config" asChild globals={{ ...desktop, theme: 'dark' }}>
  <SettingsPage sections={settingsSections} current="indexers" title="Dock Feeds" managed="indexers.dock_feeds" managedSource="corsarr.yaml, line 42"
    schema={torznabSchema} values={{ ...torznabValues, name: 'Dock Feeds', url: 'https://dock.example.net/torznab' }} meta={torznabManagedMeta} now={NOW}>
    {#snippet managedActions()}<Button size="sm" icon="copy">Copy as YAML</Button>{/snippet}
  </SettingsPage>
</Story>

<Story name="Empty section list" asChild globals={desktop}>
  <SettingsPage sections={settingsSections} current="connections" title="Connections">
    {#snippet actions()}<Button variant="primary" icon="plus">Add connection</Button>{/snippet}
    <EmptyState title="No connections yet" icon="bell" level={2}>Connect a media server to refresh it after imports, or a notification service to hear about grabs and problems.</EmptyState>
  </SettingsPage>
</Story>

<Story name="Phone editor" asChild globals={phone}>
  <SettingsPage sections={settingsSections} current="indexers" title="Harbour Tracker" schema={torznabSchema} values={torznabValues} meta={torznabMeta} test={testSlot} now={NOW} />
</Story>
