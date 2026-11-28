<!--
  Settings › Profiles (routed): loads profiles, formats and the editor choices, refreshes on
  quality and config changes, and turns the view's actions into API calls.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { ApiError } from '../../api/client';
  import type { FormatRow, ProfileDoc, ProfileRow, QualityChoices } from '../../api/types';
  import ProfilesView from './ProfilesView.svelte';
  import { refusal, type Refused } from './quality';

  const { api, live, toasts } = app();

  const rows = new Resource((s) => api.list<ProfileRow>('qualityprofiles', undefined, s));
  const formats = new Resource((s) => api.list<FormatRow>('customformats', undefined, s));
  const choices = new Resource((s) => api.get<QualityChoices>('quality/choices', undefined, s));

  onMount(() => {
    void rows.load();
    void formats.load();
    void choices.load();
    return refetchOn(live, ['quality.changed', 'config.applied'], () => { void rows.load(true); void formats.load(true); });
  });

  async function onsave(doc: ProfileDoc, isNew: boolean): Promise<Refused | void> {
    try {
      if (isNew) await api.post('qualityprofiles', doc);
      else await api.put(`qualityprofiles/${encodeURIComponent(doc.key)}`, doc);
      toasts.push({ tone: 'ok', message: `Saved ${doc.name}` });
      await rows.load(true);
    } catch (e) {
      if (e instanceof ApiError) return refusal(e.body, e.message);
      throw e;
    }
  }

  async function ondelete(key: string): Promise<string | void> {
    try {
      await api.del(`qualityprofiles/${encodeURIComponent(key)}`);
      await rows.load(true);
    } catch (e) {
      if (e instanceof ApiError) return e.message;
      throw e;
    }
  }

  async function onduplicate(r: ProfileRow): Promise<{ key?: string; message?: string }> {
    try {
      const row = await api.post<ProfileRow>(`qualityprofiles/${encodeURIComponent(r.key)}/duplicate`, { kind: r.kinds[0] });
      await rows.load(true);
      return { key: row.key };
    } catch (e) {
      if (e instanceof ApiError) return { message: e.message };
      throw e;
    }
  }

  const failed = $derived(rows.error ?? formats.error ?? choices.error);
</script>

<ProfilesView rows={rows.data ?? []} formats={formats.data ?? []} choices={choices.data} loading={rows.loading && !rows.data}
  error={failed ? { message: failed.message, label: failed.label } : undefined}
  onretry={() => { void rows.load(); void formats.load(); void choices.load(); }} {onsave} {ondelete} {onduplicate} />
