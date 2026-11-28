<!--
  Settings › Catalogue (routed): loads the catalogue (Refresh forces a fetch), installs or updates
  the exact version shown and opens the module's page once it is installed.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { ApiError } from '../../api/client';
  import type { CatalogueOffer, CatalogueView, ModuleInfo } from '../../api/types';
  import PluginCatalogueView from './PluginCatalogueView.svelte';
  import { changeError } from './plugin-modules';

  const { api, live, status, toasts, navigate } = app();
  let refresh = false;
  const cat = new Resource((s) => api.get<CatalogueView>('plugins/catalogue', refresh ? { refresh: 1 } : undefined, s));
  let refreshing = $state(false);

  onMount(() => {
    void cat.load();
    if (!status.data) void status.load();
    return refetchOn(live, ['plugin.changed'], () => void cat.load(true));
  });

  async function onrefresh() {
    refreshing = true;
    refresh = true;
    try {
      await cat.load(true);
    } finally {
      refresh = false;
      refreshing = false;
    }
  }

  async function oninstall(o: CatalogueOffer): Promise<string | void> {
    const { id, version, name } = o.entry;
    try {
      const m = o.installed
        ? await api.post<ModuleInfo>(`plugins/modules/${encodeURIComponent(id)}/update`, { version })
        : await api.post<ModuleInfo>('plugins/modules', { id, version });
      toasts.push({ tone: 'ok', message: o.installed ? `Updated ${name} to ${version}` : `Installed ${name} ${version}` });
      navigate(`/settings/plugins/modules/${encodeURIComponent(m?.id ?? id)}`);
    } catch (e) {
      if (e instanceof ApiError) {
        if (e.code === 'catalogue_changed') void cat.load(true);
        return changeError(e.code, e.message);
      }
      throw e;
    }
  }
</script>

<PluginCatalogueView catalogue={cat.data} source={status.data?.configSource} loading={cat.loading && !cat.data} {refreshing}
  error={cat.error ? { message: cat.error.message, label: cat.error.label } : undefined} onretry={cat.retry} {onrefresh} {oninstall} />
