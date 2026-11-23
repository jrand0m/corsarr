<!-- Page composition (story only): Settings › General (root folders, API key, media kind modes, server). -->
<script lang="ts">
  import Shell from './Shell.svelte';
  import SettingsPage from '../lib/components/templates/SettingsPage/SettingsPage.svelte';
  import Button from '../lib/components/atoms/Button/Button.svelte';
  import IconButton from '../lib/components/atoms/IconButton/IconButton.svelte';
  import CopyableValue from '../lib/components/atoms/CopyableValue/CopyableValue.svelte';
  import Meter from '../lib/components/atoms/Meter/Meter.svelte';
  import MediaKindIcon from '../lib/components/atoms/MediaKindIcon/MediaKindIcon.svelte';
  import Banner from '../lib/components/molecules/Banner/Banner.svelte';
  import PathField from '../lib/components/molecules/PathField/PathField.svelte';
  import SegmentedControl from '../lib/components/molecules/SegmentedControl/SegmentedControl.svelte';
  import SchemaForm from '../lib/components/organisms/SchemaForm/SchemaForm.svelte';
  import ConfirmDialog from '../lib/components/organisms/ConfirmDialog/ConfirmDialog.svelte';
  import { formatBytes } from '../lib/format/quantity';
  import type { MediaKind } from '../lib/tokens';
  import { apiKey, generalSchema, generalValues, NOW, rootFolders, settingsSections } from '../lib/storybook/fixtures';

  let { regenerate = false }: { regenerate?: boolean } = $props();

  // svelte-ignore state_referenced_locally
  let confirmOpen = $state(regenerate);
  let newFolder = $state('');
  const kindModes: { kind: MediaKind; label: string; mode: string; help: string }[] = [
    { kind: 'tv', label: 'TV', mode: 'active', help: 'Searches, grabs and imports on its own.' },
    { kind: 'movie', label: 'Movies', mode: 'active', help: 'Searches, grabs and imports on its own.' },
    { kind: 'music', label: 'Music', mode: 'shadow', help: 'Decides and logs what it would grab; changes nothing yet.' },
    { kind: 'book', label: 'Books', mode: 'disabled', help: 'Hidden from the library and never searched.' },
  ];
  const modes = [{ value: 'disabled', label: 'Disabled' }, { value: 'shadow', label: 'Shadow' }, { value: 'active', label: 'Active' }];
</script>

<Shell path="/settings">
  <SettingsPage sections={settingsSections} current="general" title="General" description="Where media lives, how other tools reach corsarr, and which kinds it manages.">
    <section class="block" aria-labelledby="sg-roots">
      <h2 id="sg-roots">Root folders</h2>
      <ul class="roots">
        {#each rootFolders as f (f.path)}
          <li class="root">
            <div class="root-main">
              <span class="path">{f.path}</span>
              <span class="meta">{f.label} · {formatBytes(f.free ?? 0)} free of {formatBytes(f.total ?? 0)}</span>
            </div>
            <span class="meter"><Meter label="Space used on {f.path}" value={1 - (f.free ?? 0) / (f.total ?? 1)} warnAt={0.85} errorAt={0.95} /></span>
            <IconButton icon="trash" label="Remove {f.path}" />
          </li>
        {/each}
      </ul>
      <div class="add">
        <div class="add-path"><PathField label="Add a root folder" help="Folders the server can see; corsarr checks it can write there." bind:value={newFolder} locations={[{ path: '/data/media/music', label: 'Music share', free: 920e9, total: 4e12, access: 'writable' }]} placeholder="/data/media/music" /></div>
        <Button icon="plus" disabled={!newFolder}>Add folder</Button>
      </div>
    </section>

    <section class="block" aria-labelledby="sg-key">
      <h2 id="sg-key">API key</h2>
      <p class="help">Bots, request tools and scripts sign in with this key. Keep it out of screenshots and chat.</p>
      <div class="key">
        <CopyableValue value={apiKey} label="API key" secret />
        <Button icon="refresh" onclick={() => (confirmOpen = true)}>Regenerate…</Button>
      </div>
    </section>

    <section class="block" aria-labelledby="sg-kinds">
      <h2 id="sg-kinds">Media kinds</h2>
      <Banner preset="shadow">Music is in shadow mode: corsarr logs what it would grab and import, and changes nothing until you switch it to Active.</Banner>
      <ul class="kinds">
        {#each kindModes as k (k.kind)}
          <li class="kind">
            <span class="kind-name"><MediaKindIcon kind={k.kind} /><span>{k.label}</span></span>
            <span class="kind-help">{k.help}</span>
            <span class="kind-mode"><SegmentedControl label="{k.label} mode" options={modes} value={k.mode} size="sm" /></span>
          </li>
        {/each}
      </ul>
    </section>

    <section class="block" aria-labelledby="sg-server">
      <h2 id="sg-server">Server</h2>
      <div class="editor"><SchemaForm schema={generalSchema} values={generalValues} label="Server settings" onsave={() => {}} now={NOW} /></div>
    </section>
  </SettingsPage>

  <ConfirmDialog bind:open={confirmOpen} title="Regenerate the API key?" confirmLabel="Regenerate key" undoable={false}
    consequences={['The current key stops working at once.', 'Bots, request tools and scripts using it lose access until you paste the new key into them.']}
    onconfirm={() => (confirmOpen = false)} oncancel={() => (confirmOpen = false)} />
</Shell>

<style>
  .block { display: grid; gap: var(--space-3); min-width: 0; }
  h2 { font-size: var(--text-lg); margin-top: var(--space-2); }
  .help { color: var(--ink-muted); font-size: var(--text-md); max-width: var(--measure); }
  .roots, .kinds { list-style: none; margin: 0; padding: 0; background: var(--surface-raised); border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); }
  .roots li + li, .kinds li + li { border-top: var(--border-w) solid var(--line); }
  .root { display: flex; align-items: center; gap: var(--space-3); padding: var(--space-2) var(--space-2) var(--space-2) var(--space-4); min-width: 0; }
  .root-main { display: grid; gap: var(--space-0-5); flex: 1; min-width: 0; }
  .path { font-family: var(--font-mono); font-size: var(--text-sm); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .meta { font-size: var(--text-sm); color: var(--ink-muted); }
  .meter { width: 8rem; flex: none; }
  .add { display: flex; align-items: flex-end; gap: var(--space-2); flex-wrap: wrap; }
  .add-path { flex: 1 1 18rem; min-width: 0; }
  .key { display: flex; align-items: center; gap: var(--space-2); flex-wrap: wrap; min-width: 0; }
  .key > :global(:first-child) { flex: 1 1 16rem; min-width: 0; }
  .kind { display: grid; grid-template-columns: 8rem minmax(0, 1fr) auto; align-items: center; gap: var(--space-2) var(--space-4); padding: var(--space-3) var(--space-4); }
  .kind-name { display: flex; align-items: center; gap: var(--space-2); font-weight: var(--weight-medium); }
  .kind-help { font-size: var(--text-md); color: var(--ink-muted); }
  .editor { padding: var(--space-5); background: var(--surface-raised); border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); min-width: 0; }
  @media (width < 48rem) {
    .meter { display: none; }
    .kind { grid-template-columns: minmax(0, 1fr); padding: var(--space-3); }
    .editor { padding: var(--space-4) var(--space-3); }
  }
</style>
