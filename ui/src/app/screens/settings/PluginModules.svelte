<!--
  Settings › Installed modules (routed): loads the modules and the policy, refetches on module
  changes and turns rollback and uninstall into API calls.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { ApiError } from '../../api/client';
  import type { ModuleInfo, ModuleList } from '../../api/types';
  import PluginModulesView from './PluginModulesView.svelte';
  import { changeError } from './plugin-modules';

  const { api, live, status, toasts, navigate } = app();
  const list = new Resource((s) => api.get<ModuleList>('plugins/modules', undefined, s));

  onMount(() => {
    void list.load();
    if (!status.data) void status.load();
    const off = refetchOn(live, ['plugin.changed', 'plugin.loaded', 'config.applied'], () => void list.load(true));
    return off;
  });

  async function onchange(change: 'rollback' | 'uninstall', m: ModuleInfo): Promise<string | void> {
    const id = encodeURIComponent(m.id);
    try {
      if (change === 'rollback') await api.post(`plugins/modules/${id}/rollback`);
      else await api.del(`plugins/modules/${id}`);
      toasts.push({ tone: 'ok', message: change === 'rollback' ? `Rolled back ${m.manifest.name || m.id}` : `Uninstalled ${m.manifest.name || m.id}` });
      await list.load(true);
    } catch (e) {
      if (e instanceof ApiError) return changeError(e.code, e.message);
      throw e;
    }
  }
</script>

<PluginModulesView modules={list.data?.items ?? []} policy={list.data?.policy} source={status.data?.configSource}
  loading={list.loading && !list.data} error={list.error ? { message: list.error.message, label: list.error.label } : undefined}
  onretry={list.retry} {onchange} onopen={(m) => navigate(`/settings/plugins/modules/${encodeURIComponent(m.id)}`)} />
