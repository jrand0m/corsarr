<!-- Page composition (story only): Settings › Installed modules with fixture modules and
     in-memory rollback and uninstall; `locked` shows the config lock. -->
<script lang="ts">
  import Shell from './Shell.svelte';
  import PluginModulesView from '../app/screens/settings/PluginModulesView.svelte';
  import type { ModuleInfo } from '../app/api/types';
  import { modules, policyLocked, policyOpen } from '../lib/storybook/plugin-module-fixtures';

  let { view = 'list' }: { view?: 'list' | 'locked' | 'empty' | 'loading' | 'error' } = $props();
  // svelte-ignore state_referenced_locally
  let rows = $state<ModuleInfo[]>(view === 'empty' ? [] : modules.map((m) => ({ ...m })));

  async function onchange(change: 'rollback' | 'uninstall', m: ModuleInfo) {
    await new Promise((r) => setTimeout(r, 600));
    rows = rows.map((r) => (r.id !== m.id ? r : change === 'rollback' ? { ...r, version: r.previous ?? r.version, previous: undefined } : { ...r, status: 'uninstalled' as const }));
  }
</script>

<Shell path="/settings">
  <PluginModulesView modules={view === 'loading' || view === 'error' ? [] : rows} policy={view === 'locked' ? policyLocked : policyOpen}
    source={{ kind: 'file', path: '/config/corsarr.yml' }} loading={view === 'loading'}
    error={view === 'error' ? { message: 'corsarr is not answering. Check that the server is running and reachable.', label: 'network' } : undefined}
    onretry={() => {}} {onchange} />
</Shell>
