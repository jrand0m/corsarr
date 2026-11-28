<!-- Page composition (story only): one installed module with its manifest, permissions, a
     configured instance (read-only, actions answer from memory) and the new-instance draft. -->
<script lang="ts">
  import Shell from './Shell.svelte';
  import PluginModuleView from '../app/screens/settings/PluginModuleView.svelte';
  import type { ActionReply } from '../lib/components/organisms/PluginSettingsForm/PluginSettingsForm.svelte';
  import { harbourEnvExport, harbourModule, modules, policyLocked, policyOpen } from '../lib/storybook/plugin-module-fixtures';
  import { instanceViews } from '../app/screens/settings/plugin-modules';
  import { parseEnvExport } from '../app/model';

  let { view = 'detail' }: { view?: 'detail' | 'locked' | 'failed' | 'missing' | 'loading' } = $props();
  const instances = instanceViews(harbourModule, parseEnvExport(harbourEnvExport).instances);
  const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));

  async function onaction(action: string): Promise<ActionReply> {
    await wait(700);
    return action === 'test' ? { outcome: 'ok', text: 'Connected to Harbour 2.4 ✅' } : { outcome: 'warn', text: 'Found 2 categories.\nThe category "corsarr" is missing ⚠️' };
  }
  async function onvalidate(values: Record<string, unknown>) {
    await wait(400);
    return values.url ? [] : [{ path: 'url', message: 'is required' }];
  }
  const module = $derived(view === 'missing' || view === 'loading' ? undefined : view === 'failed' ? modules[1] : harbourModule);
</script>

<Shell path="/settings">
  <PluginModuleView id={view === 'missing' ? 'gonedl' : (module?.id ?? 'harbourdl')} {module} instances={view === 'detail' || view === 'locked' ? instances : []}
    policy={view === 'locked' ? policyLocked : policyOpen} source={{ kind: 'env' }} loading={view === 'loading'}
    {onaction} {onvalidate} onchange={async () => { await wait(600); }} />
</Shell>
