<!--
  Settings › Indexers (routed): loads indexers, plugins, definitions and the Torznab state,
  refreshes on health and plugin events, and turns the view's actions into API calls.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { ApiError } from '../../api/client';
  import type { DefinitionEntry, IndexerInput, IndexerRow, IndexerTest, PluginManifest, TorznabState } from '../../api/types';
  import IndexersView, { type Refusal } from './IndexersView.svelte';
  import { fieldErrors, refusedTest } from './indexers';

  const { api, live, toasts } = app();

  const rows = new Resource((s) => api.list<IndexerRow>('indexers', undefined, s));
  const plugins = new Resource((s) => api.list<PluginManifest>('plugins', undefined, s));
  const defs = new Resource((s) => api.get<{ items: DefinitionEntry[] | null; directory: string }>('indexers/definitions', undefined, s));
  const torznab = new Resource((s) => api.get<TorznabState>('indexers/torznab', undefined, s));

  onMount(() => {
    void rows.load();
    void plugins.load();
    void defs.load();
    void torznab.load();
    const offs = [
      refetchOn(live, ['health.changed'], () => void rows.load(true)),
      refetchOn(live, ['config.applied', 'plugin.loaded'], () => { void rows.load(true); void plugins.load(true); void defs.load(true); }),
    ];
    return () => offs.forEach((off) => off());
  });

  /** A 422 becomes field messages or the failed test; anything else is thrown to the form. */
  async function attempt(fn: () => Promise<unknown>): Promise<Refusal | void> {
    try {
      await fn();
      await rows.load(true);
    } catch (e) {
      if (e instanceof ApiError && e.status === 422) {
        const test = refusedTest(e.body);
        return test ? { test } : { fields: fieldErrors(e.body), message: e.message };
      }
      if (e instanceof ApiError) return { message: e.message };
      throw e;
    }
  }

  const onadd = (input: IndexerInput) => attempt(async () => {
    await api.post('indexers', input);
    toasts.push({ tone: 'ok', message: `Added ${input.name ?? input.key}` });
  });
  const onedit = (key: string, input: IndexerInput) => attempt(() => api.put(`indexers/${encodeURIComponent(key)}`, input));
  const ontest = async (key: string) => {
    const r = await api.post<IndexerTest>(`indexers/${encodeURIComponent(key)}/test`);
    void rows.load(true);
    return r;
  };
  async function ondelete(key: string) {
    await api.del(`indexers/${encodeURIComponent(key)}`);
    await rows.load(true);
  }
  async function onreload() {
    const r = await api.post<{ items: DefinitionEntry[] | null; directory: string }>('indexers/definitions/reload');
    defs.data = r;
    void plugins.load(true);
    void rows.load(true);
  }
  async function ontorznab(on: boolean) {
    torznab.data = await api.put<TorznabState>('indexers/torznab', { enabled: on });
  }

  const origin = typeof location === 'undefined' ? '' : location.origin;
  const failed = $derived(rows.error ?? plugins.error);
</script>

<IndexersView rows={rows.data ?? []} manifests={plugins.data ?? []} definitions={defs.data?.items ?? []} directory={defs.data?.directory ?? ''}
  torznab={torznab.data} {origin} loading={rows.loading && !rows.data} error={failed ? { message: failed.message, label: failed.label } : undefined}
  onretry={() => { void rows.load(); void plugins.load(); }} {onadd} {onedit} {ontest} {ondelete} {onreload} {ontorznab} />
