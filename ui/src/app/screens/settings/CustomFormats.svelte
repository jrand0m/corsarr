<!--
  Settings › Custom formats (routed): loads formats and the condition choices, refreshes on
  quality and config changes, and turns the view's actions into API calls.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { ApiError } from '../../api/client';
  import type { FormatDoc, FormatRow, FormatTestResult, QualityChoices } from '../../api/types';
  import CustomFormatsView from './CustomFormatsView.svelte';
  import { refusal, type Refused } from './quality';

  const { api, live, toasts } = app();

  const rows = new Resource((s) => api.list<FormatRow>('customformats', undefined, s));
  const choices = new Resource((s) => api.get<QualityChoices>('quality/choices', undefined, s));

  onMount(() => {
    void rows.load();
    void choices.load();
    const off = refetchOn(live, ['quality.changed', 'config.applied'], () => void rows.load(true));
    return off;
  });

  async function onsave(doc: FormatDoc, isNew: boolean): Promise<Refused | void> {
    try {
      if (isNew) await api.post('customformats', doc);
      else await api.put(`customformats/${encodeURIComponent(doc.key)}`, doc);
      toasts.push({ tone: 'ok', message: `Saved ${doc.name}` });
      await rows.load(true);
    } catch (e) {
      if (e instanceof ApiError) return refusal(e.body, e.message);
      throw e;
    }
  }

  async function ondelete(key: string): Promise<string | void> {
    try {
      await api.del(`customformats/${encodeURIComponent(key)}`);
      await rows.load(true);
    } catch (e) {
      if (e instanceof ApiError) return e.message;
      throw e;
    }
  }

  async function ontest(title: string, draft: FormatDoc): Promise<FormatTestResult | undefined> {
    try {
      const r = await api.post<{ results: FormatTestResult[] }>('customformats/test', { titles: [title], format: draft });
      return r.results[0];
    } catch (e) {
      if (e instanceof ApiError && e.status === 422) return undefined;
      throw e;
    }
  }

  const failed = $derived(rows.error ?? choices.error);
</script>

<CustomFormatsView rows={rows.data ?? []} kinds={choices.data?.conditionKinds ?? []} loading={rows.loading && !rows.data}
  error={failed ? { message: failed.message, label: failed.label } : undefined} onretry={() => { void rows.load(); void choices.load(); }}
  {onsave} {ondelete} {ontest} />
