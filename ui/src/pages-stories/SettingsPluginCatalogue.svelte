<!-- Page composition (story only): the module catalogue with every offer state, in-memory
     install and update, and the stale, failed and locked variants. -->
<script lang="ts">
  import Shell from './Shell.svelte';
  import PluginCatalogueView from '../app/screens/settings/PluginCatalogueView.svelte';
  import type { CatalogueOffer, CatalogueView } from '../app/api/types';
  import { catalogue, catalogueFailed, catalogueStale, policyLocked } from '../lib/storybook/plugin-module-fixtures';

  let { view = 'list' }: { view?: 'list' | 'stale' | 'failed' | 'locked' | 'refused' | 'loading' } = $props();
  const now = new Date('2026-11-27T10:00:00Z');
  const pick = (): CatalogueView | undefined =>
    view === 'stale' ? catalogueStale : view === 'failed' ? catalogueFailed : view === 'locked' ? { ...catalogue, policy: policyLocked } : view === 'loading' ? undefined : catalogue;
  // svelte-ignore state_referenced_locally
  let data = $state(pick());

  async function oninstall(o: CatalogueOffer) {
    await new Promise((r) => setTimeout(r, 800));
    if (view === 'refused') return 'The catalogue now offers another version. Check the new entry and confirm again.';
    data = data && { ...data, offers: (data.offers ?? []).map((x) => (x.entry.id === o.entry.id ? { ...x, state: 'installed' as const, installed: o.entry.version } : x)) };
  }
</script>

<Shell path="/settings">
  <PluginCatalogueView catalogue={data} source={{ kind: 'file', path: '/config/corsarr.yml' }} loading={view === 'loading'} {now} onretry={() => {}} onrefresh={() => {}} {oninstall} />
</Shell>
