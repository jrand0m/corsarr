<!-- Page composition (story only): the shipped Settings › Indexers screen with fixture data and
     in-memory actions: list, add with test-on-save, view a config indexer, test results, catalogue. -->
<script lang="ts">
  import Shell from './Shell.svelte';
  import IndexersView from '../app/screens/settings/IndexersView.svelte';
  import type { IndexerInput, IndexerRow, IndexerTest } from '../app/api/types';
  import { definitionCatalogue, failedTest, indexerList, indexerManifests, passedTest } from '../lib/storybook/indexer-fixtures';

  interface Props {
    view?: 'list' | 'empty' | 'add' | 'managed' | 'tested' | 'shared';
  }
  let { view = 'list' }: Props = $props();

  // svelte-ignore state_referenced_locally
  let rows = $state<IndexerRow[]>(view === 'empty' ? [] : indexerList.map((r) => ({ ...r })));
  // svelte-ignore state_referenced_locally
  let shared = $state(view === 'shared');

  async function onadd(input: IndexerInput) {
    if (input.plugin === 'harbourmusic') return { test: failedTest };
    const m = indexerManifests.find((x) => x.id === input.plugin)!;
    rows = [...rows, { key: input.key ?? 'new', name: input.name ?? input.key ?? 'new', plugin: m.id, pluginName: m.name, delivery: m.delivery,
      source: 'ui', enabled: true, running: true, priority: 25, settings: input.settings ?? {}, secretsSet: [], torznab: `/torznab/${input.key}/api` }];
  }
  async function onedit(key: string, input: IndexerInput) {
    rows = rows.map((r) => (r.key === key ? { ...r, enabled: input.enabled ?? r.enabled, name: input.name ?? r.name } : r));
  }
  const ontest = async (key: string): Promise<IndexerTest> => (key === 'harbour' ? failedTest : passedTest);
  async function ondelete(key: string) {
    rows = rows.filter((r) => r.key !== key);
  }
</script>

<Shell path="/settings">
  <IndexersView {rows} manifests={indexerManifests} definitions={definitionCatalogue} directory="/data/plugins/definitions"
    torznab={{ enabled: shared, all: '/torznab/all/api' }} origin="http://media-box:8989"
    initialAdd={view === 'add' ? 'knaben' : undefined} initialEdit={view === 'managed' ? 'dock' : undefined}
    initialTest={view === 'tested' ? { key: 'harbour', result: failedTest } : undefined}
    {onadd} {onedit} {ontest} {ondelete} onreload={async () => {}} ontorznab={async (on) => { shared = on; }} />
</Shell>
