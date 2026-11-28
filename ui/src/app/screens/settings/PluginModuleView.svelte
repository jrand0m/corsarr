<!--
  Settings › Plugins & integrations › Installed modules › one module (view): what the module says
  about itself (all plain text; the homepage is shown as text, never as a link), what it may touch,
  the configured instances that use it (read-only settings with the managed lock and the module's
  action buttons) and a new-instance draft the server checks before the page shows the YAML to add
  to the config. Rollback and uninstall go through PluginInstallDialog and follow the config lock.
-->
<script lang="ts">
  import SettingsPage from '../../../lib/components/templates/SettingsPage/SettingsPage.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import ManagedValue from '../../../lib/components/atoms/ManagedValue/ManagedValue.svelte';
  import Skeleton from '../../../lib/components/atoms/Skeleton/Skeleton.svelte';
  import StatusBadge from '../../../lib/components/atoms/StatusBadge/StatusBadge.svelte';
  import Banner from '../../../lib/components/molecules/Banner/Banner.svelte';
  import CodeViewer from '../../../lib/components/molecules/CodeViewer/CodeViewer.svelte';
  import EmptyState from '../../../lib/components/molecules/EmptyState/EmptyState.svelte';
  import ErrorState from '../../../lib/components/molecules/ErrorState/ErrorState.svelte';
  import PermissionList from '../../../lib/components/molecules/PermissionList/PermissionList.svelte';
  import PropertyList, { type Property } from '../../../lib/components/molecules/PropertyList/PropertyList.svelte';
  import TextField from '../../../lib/components/molecules/TextField/TextField.svelte';
  import PluginInstallDialog from '../../../lib/components/organisms/PluginInstallDialog/PluginInstallDialog.svelte';
  import PluginSettingsForm, { type ActionReply } from '../../../lib/components/organisms/PluginSettingsForm/PluginSettingsForm.svelte';
  import { configSnippet, sectionFor } from '../../../lib/forms/plugin-schema';
  import type { ModuleInfo, PluginPolicy, Problem } from '../../api/types';
  import { fieldLabels, lockOf, type InstanceView } from './plugin-modules';
  import { sectionLabel } from './plugin-form';
  import { settingsSections } from './sections';

  type Change = 'rollback' | 'uninstall';
  interface Props {
    id: string;
    module?: ModuleInfo;
    instances?: InstanceView[];
    policy?: PluginPolicy;
    source?: { kind?: string; path?: string };
    loading?: boolean;
    error?: { message: string; label: string };
    onretry?: () => void;
    /** Runs a module action for a configured instance ("section.key") or the draft. */
    onaction?: (action: string, instance: string | undefined, values: Record<string, unknown>) => Promise<ActionReply>;
    /** Server check of draft values; resolves with its problems. */
    onvalidate?: (values: Record<string, unknown>) => Promise<Problem[]>;
    onchange?: (change: Change, m: ModuleInfo) => Promise<string | void>;
  }
  let { id, module: m, instances = [], policy, source, loading = false, error, onretry, onaction, onvalidate, onchange }: Props = $props();

  const lock = $derived(lockOf(policy, source));
  const name = $derived(m?.manifest.name || m?.id || id);
  const labels = $derived(m ? fieldLabels(m) : {});
  const details = $derived.by((): Property[] => {
    if (!m) return [];
    const man = m.manifest;
    const lim = man.limits ?? {};
    const limits = [lim.memoryMiB ? `${lim.memoryMiB} MiB` : '', lim.callTimeoutMs ? `${lim.callTimeoutMs / 1000} s per call` : '', lim.concurrency ? `${lim.concurrency} at once` : ''].filter(Boolean).join(' · ');
    return [
      { key: 'version', label: 'Version', value: m.previous ? `${m.version} (previous ${m.previous})` : m.version, mono: true },
      { key: 'kind', label: 'Kind', value: m.kind },
      { key: 'author', label: 'Author', value: man.author },
      { key: 'license', label: 'License', value: man.license },
      { key: 'homepage', label: 'Homepage', value: man.homepage, mono: true },
      { key: 'source', label: 'Installed from', value: m.source === 'catalogue' ? 'The catalogue' : 'A file' },
      { key: 'sha', label: 'SHA-256', value: m.sha256, mono: true, copy: true },
      { key: 'limits', label: 'Asks for', value: limits || undefined },
    ];
  });

  // Dialog for rollback and uninstall.
  let pending = $state<Change>();
  let open = $state(false);
  let busy = $state(false);
  let refusal = $state<string>();
  function ask(c: Change) {
    pending = c;
    refusal = undefined;
    open = true;
  }
  async function confirm() {
    if (!m || !pending || !onchange) return;
    busy = true;
    try {
      const r = await onchange(pending, m);
      if (r) refusal = r;
      else open = false;
    } finally {
      busy = false;
    }
  }

  // New-instance draft.
  let draftKey = $state('');
  let draftName = $state('');
  let keyError = $state<string>();
  let snippet = $state<string>();
  const identRe = /^[a-z][a-zA-Z0-9]{0,47}$/;
  async function check(values: Record<string, unknown>): Promise<Problem[]> {
    snippet = undefined;
    keyError = identRe.test(draftKey) ? undefined : 'Use a lower-case letter followed by letters or digits, for example upstairs';
    const problems = (await onvalidate?.(values)) ?? [];
    if (!problems.length && !keyError && m) {
      snippet = configSnippet({ section: sectionFor(m.kind), key: draftKey, plugin: m.id, name: draftName.trim() || undefined, fields: m.manifest.settings.fields, values });
    }
    return problems;
  }
</script>

<SettingsPage sections={settingsSections()} current="plugins" title={name} description={m ? `${m.kind} module · ${m.version}` : undefined}>
  {#snippet actions()}
    <Button icon="arrow-left" href="#/settings/plugins/modules">Installed modules</Button>
    {#if m && m.status !== 'uninstalled'}
      <Button icon="undo" disabled={!!lock || !m.previous} onclick={() => ask('rollback')}>Roll back</Button>
      <Button icon="trash" variant="danger" disabled={!!lock} onclick={() => ask('uninstall')}>Uninstall</Button>
    {/if}
  {/snippet}
  {#snippet banner()}
    {#if lock}
      <Banner preset="managed" title="Module changes are off">
        Installing, updating and removing modules is locked by <ManagedValue value="plugins.uiInstall" managed={lock} />. Actions and checks still work.
      </Banner>
    {/if}
  {/snippet}

  {#if loading && !m}
    <Skeleton shape="block" height="16rem" />
  {:else if error}
    <ErrorState title="Couldn't load the module" code={error.label} {onretry} level={2}>{error.message}</ErrorState>
  {:else if !m}
    <EmptyState title="No module called {id}" icon="plug" level={2}>
      It may have been removed.
      {#snippet actions()}<Button href="#/settings/plugins/modules">See installed modules</Button>{/snippet}
    </EmptyState>
  {:else}
    <section class="block" aria-labelledby="pm-about">
      <div class="row">
        <h2 id="pm-about">About</h2>
        <StatusBadge family="module" value={m.status} size="sm" />
      </div>
      {#if m.error}<Banner tone="error" icon="alert-octagon" title="The module did not load">{m.error}</Banner>{/if}
      {#if m.manifest.description}<p class="desc">{m.manifest.description}</p>{/if}
      <PropertyList items={details} label="Module details" />
    </section>

    <section class="block" aria-labelledby="pm-perms">
      <h2 id="pm-perms">May touch</h2>
      <div class="card"><PermissionList permissions={m.manifest.permissions} {labels} /></div>
    </section>

    <section class="block" aria-labelledby="pm-inst">
      <h2 id="pm-inst">Instances using it</h2>
      {#if instances.length}
        {#each instances as inst (`${inst.ref.section}.${inst.ref.key}`)}
          {@const ref = `${inst.ref.section}.${inst.ref.key}`}
          <article class="card inst" aria-label={inst.ref.name || inst.ref.key}>
            <header class="row">
              <h3>{inst.ref.name || inst.ref.key}</h3>
              <span class="meta">{sectionLabel(inst.ref.section)} · <span class="mono">{ref}</span>{inst.ref.enabled ? '' : ' · switched off'}</span>
            </header>
            <PluginSettingsForm schema={m.manifest.settings} values={inst.values} meta={inst.meta} label="{inst.ref.name || inst.ref.key} settings" readonly
              onaction={onaction ? (a, v) => onaction(a, ref, v) : undefined} />
          </article>
        {/each}
      {:else}
        <p class="muted">No configured instance uses {name} yet. Start one below.</p>
      {/if}
    </section>

    <section class="block" aria-labelledby="pm-new">
      <h2 id="pm-new">New instance</h2>
      <p class="help">Fill in the settings and check them. corsarr validates them with the module's rules and shows the YAML to add under <span class="mono">{sectionFor(m.kind)}</span> in the config; secrets become environment references.</p>
      <div class="card draft">
        <div class="names">
          <TextField label="Key" required mono help="Its name in the config, for example upstairs." error={keyError} bind:value={draftKey} />
          <TextField label="Display name" help="Shown in lists and History." bind:value={draftName} />
        </div>
        <PluginSettingsForm schema={m.manifest.settings} label="New {name} instance" submitLabel="Check and show YAML" onsubmit={check}
          onaction={onaction ? (a, v) => onaction(a, undefined, v) : undefined} />
        {#if snippet}
          <CodeViewer code={snippet} language="yaml" filename="{draftKey}.yaml" label="Config for the new instance" />
        {/if}
      </div>
    </section>
  {/if}
</SettingsPage>

{#if m && pending}
  <PluginInstallDialog bind:open change={pending} {name} version={pending === 'rollback' ? m.previous : undefined} installed={m.version}
    usedBy={m.usedBy ?? []} {labels} {lock} {busy} error={refusal} onconfirm={confirm} />
{/if}

<style>
  .block { display: grid; gap: var(--space-3); min-width: 0; }
  .row { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); flex-wrap: wrap; }
  h2 { font-size: var(--text-lg); margin-top: var(--space-2); }
  h3 { font-size: var(--text-md); font-weight: var(--weight-semibold); }
  .desc { white-space: pre-line; max-width: var(--measure); color: var(--ink); overflow-wrap: anywhere; }
  .help, .muted { color: var(--ink-muted); font-size: var(--text-md); max-width: var(--measure); }
  .meta { color: var(--ink-muted); font-size: var(--text-sm); }
  .mono { font-family: var(--font-mono); font-size: var(--text-sm); }
  .card { padding: var(--space-5); background: var(--surface-raised); border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); min-width: 0; }
  .inst, .draft { display: grid; gap: var(--space-4); }
  .names { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 18rem), 1fr)); gap: var(--space-5) var(--space-6); max-width: 48rem; }
  @media (width < 48rem) { .card { padding: var(--space-4) var(--space-3); } }
</style>
