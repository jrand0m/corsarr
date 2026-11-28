<!-- Page composition (story only): the shipped Settings › Profiles and Settings › Custom formats
     screens with the owner's device formats and tiered profiles and in-memory actions. -->
<script lang="ts">
  import Shell from './Shell.svelte';
  import CustomFormatsView from '../app/screens/settings/CustomFormatsView.svelte';
  import ProfilesView from '../app/screens/settings/ProfilesView.svelte';
  import type { FormatDoc, FormatRow, ProfileDoc, ProfileRow } from '../app/api/types';
  import { formatRows, formatTest, profileRows, qualityChoices } from '../lib/storybook/quality-fixtures';

  interface Props {
    screen?: 'profiles' | 'formats';
    view?: 'list' | 'edit' | 'builtin' | 'new' | 'empty';
  }
  let { screen = 'profiles', view = 'list' }: Props = $props();

  // svelte-ignore state_referenced_locally
  let formats = $state<FormatRow[]>(view === 'empty' ? [] : formatRows.map((r) => ({ ...r })));
  // svelte-ignore state_referenced_locally
  let profiles = $state<ProfileRow[]>(profileRows.map((r) => ({ ...r })));

  async function saveFormat(doc: FormatDoc, isNew: boolean) {
    const row: FormatRow = { ...doc, key: doc.key || doc.name.toLowerCase().replace(/[^a-z0-9]/g, ''), origin: 'database', usedBy: [] };
    formats = isNew ? [...formats, row] : formats.map((f) => (f.key === row.key ? { ...f, ...row } : f));
  }
  async function saveProfile(doc: ProfileDoc, isNew: boolean) {
    const row: ProfileRow = { ...doc, key: doc.key || doc.name.toLowerCase().replace(/[^a-z0-9]/g, ''), builtin: false, origin: 'database', items: 0 };
    profiles = isNew ? [...profiles, row] : profiles.map((p) => (p.key === row.key ? { ...p, ...row } : p));
  }
  async function duplicate(r: ProfileRow) {
    const key = `${r.key}copy`;
    profiles = [...profiles, { ...r, key, name: `${r.name} (copy)`, builtin: false, origin: 'database', items: 0 }];
    return { key };
  }
  const initialProfile = $derived(view === 'edit' ? 'tv:highps5' : view === 'builtin' ? 'tv:hd1080p' : undefined);
  const initialFormat = $derived(view === 'edit' ? 'ps5audio' : view === 'new' ? '' : undefined);
</script>

<Shell path="/settings">
  {#if screen === 'profiles'}
    <ProfilesView rows={profiles} {formats} choices={qualityChoices} initialEdit={initialProfile}
      onsave={saveProfile} ondelete={async (k) => (profiles.find((p) => p.key === k)?.items ? 'used by 42 series, movies or artists; give them another profile first' : undefined)}
      onduplicate={duplicate} />
  {:else}
    <CustomFormatsView rows={formats} kinds={qualityChoices.conditionKinds} initialEdit={initialFormat}
      onsave={saveFormat} ondelete={async (k) => (formats.find((f) => f.key === k)?.usedBy.length ? 'used by the profiles High ~8GB/h - PS5' : undefined)}
      ontest={async (t) => formatTest(t)} />
  {/if}
</Shell>
