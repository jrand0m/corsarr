<!--
  Settings › Installed modules › one module (routed): loads the module, the policy and the config
  export (values of the instances using it), refetches on module changes, and turns actions,
  draft checks, rollback and uninstall into API calls.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { app } from '../../context.svelte';
  import { Resource } from '../../resource.svelte';
  import { refetchOn } from '../../live.svelte';
  import { ApiError } from '../../api/client';
  import type { ActionResult, ModuleInfo, ModuleList, ProblemList } from '../../api/types';
  import type { ActionReply } from '../../../lib/components/organisms/PluginSettingsForm/PluginSettingsForm.svelte';
  import { parseEnvExport } from '../../model';
  import PluginModuleView from './PluginModuleView.svelte';
  import { changeError, instanceViews, problemsOf } from './plugin-modules';
  import type { ScreenProps } from '../../screen';

  let { params = {} }: ScreenProps = $props();
  const { api, live, status, toasts, navigate } = app();
  const id = $derived(params.id ?? '');
  const path = $derived(`plugins/modules/${encodeURIComponent(id)}`);

  const mod = new Resource((s) => api.get<ModuleInfo>(path, undefined, s));
  const policy = new Resource((s) => api.get<ModuleList>('plugins/modules', undefined, s));
  const cfg = new Resource(async () => parseEnvExport(await api.text('config/export', { format: 'env', full: true })));

  onMount(() => {
    void policy.load();
    void cfg.load();
    if (!status.data) void status.load();
    const offs = [
      refetchOn(live, ['plugin.changed', 'plugin.loaded'], () => { void mod.load(true); void policy.load(true); }),
      refetchOn(live, ['config.applied'], () => { void cfg.load(true); void mod.load(true); }),
    ];
    return () => offs.forEach((off) => off());
  });
  $effect(() => {
    void path;
    void mod.load();
  });

  const notFound = $derived(mod.error instanceof ApiError && mod.error.status === 404);
  const instances = $derived(mod.data ? instanceViews(mod.data, cfg.data?.instances ?? []) : []);

  async function onaction(action: string, instance: string | undefined, values: Record<string, unknown>): Promise<ActionReply> {
    try {
      return await api.post<ActionResult>(`${path}/actions/${encodeURIComponent(action)}`, { instance, values: instance ? {} : values });
    } catch (e) {
      if (e instanceof ApiError && e.status === 422) {
        const problems = problemsOf(e.body);
        if (problems.length) return { problems };
        if (e.code === 'text_refused') return { refused: true };
      }
      throw e;
    }
  }
  async function onvalidate(values: Record<string, unknown>) {
    const r = await api.post<ProblemList>(`${path}/validate`, { values });
    return r.problems ?? [];
  }
  async function onchange(change: 'rollback' | 'uninstall', m: ModuleInfo): Promise<string | void> {
    try {
      if (change === 'rollback') mod.data = await api.post<ModuleInfo>(`${path}/rollback`);
      else await api.del(path);
      toasts.push({ tone: 'ok', message: change === 'rollback' ? `Rolled back ${m.manifest.name || m.id}` : `Uninstalled ${m.manifest.name || m.id}` });
      if (change === 'uninstall') navigate('/settings/plugins/modules');
    } catch (e) {
      if (e instanceof ApiError) return changeError(e.code, e.message);
      throw e;
    }
  }
</script>

<PluginModuleView {id} module={notFound ? undefined : mod.data} {instances} policy={policy.data?.policy} source={status.data?.configSource}
  loading={mod.loading && !mod.data} error={mod.error && !notFound ? { message: mod.error.message, label: mod.error.label } : undefined}
  onretry={mod.retry} {onaction} {onvalidate} {onchange} />
