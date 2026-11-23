<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Dialog from './Dialog.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import EnumField from '../../molecules/EnumField/EnumField.svelte';
  import TextField from '../../molecules/TextField/TextField.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const folders = [
    { value: '/srv/media/tv', label: '/srv/media/tv · 1.8 TB free' },
    { value: '/mnt/archive/tv', label: '/mnt/archive/tv · 212 GB free' },
  ];
  const profiles = [
    { value: 'hd', label: 'HD 1080p' },
    { value: 'uhd', label: 'Ultra HD' },
  ];
  const { Story } = defineMeta({
    title: 'Organisms/Dialog',
    component: Dialog,
    tags: ['autodocs'],
    args: { title: 'Add The Long Haul', description: 'TV · 2024 · 3 seasons · Harbourline', size: 'md', dismissible: true },
  });
</script>

{#snippet addForm()}
  <div class="form">
    <EnumField label="Root folder" options={folders} value="/srv/media/tv" />
    <EnumField label="Quality profile" options={profiles} value="hd" />
    <TextField label="Tags" placeholder="e.g. family, weekend" />
  </div>
{/snippet}

{#snippet addActions()}
  <Button variant="ghost">Cancel</Button>
  <Button variant="primary" icon="plus">Add and search</Button>
{/snippet}

<script lang="ts">
  let open = $state(false);
</script>

<Story name="Playground">
  {#snippet template(args)}<Dialog {...args} modal={false} open children={addForm} actions={addActions} />{/snippet}
</Story>

<Story name="Modal (focus trap, Escape, return focus)" asChild>
  <Button onclick={() => (open = true)}>Add series…</Button>
  <Dialog bind:open title="Add The Long Haul" description="TV · 2024 · 3 seasons · Harbourline">
    {@render addForm()}
    {#snippet actions()}
      <Button variant="ghost" onclick={() => (open = false)}>Cancel</Button>
      <Button variant="primary" icon="plus" onclick={() => (open = false)}>Add and search</Button>
    {/snippet}
  </Dialog>
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Form with footer actions">
      <Dialog modal={false} open title="Add The Long Haul" description="TV · 2024 · 3 seasons · Harbourline" children={addForm} actions={addActions} />
    </Row>
    <Row label="Small, no description">
      <Dialog modal={false} open size="sm" title="Rename 13 files?">
        <p>Files in Season 2 move to the naming pattern set in Import &amp; naming.</p>
        {#snippet actions()}<Button variant="ghost">Cancel</Button><Button variant="primary">Rename</Button>{/snippet}
      </Dialog>
    </Row>
    <Row label="Not dismissible (session expired)">
      <Dialog modal={false} open size="sm" dismissible={false} title="Sign in again" description="Your session ended. What you were doing stays on screen.">
        <TextField label="Password" />
        {#snippet actions()}<Button variant="primary">Sign in</Button>{/snippet}
      </Dialog>
    </Row>
    <Row label="Long content scrolls inside">
      <div class="short">
        <Dialog modal={false} open title="Parse details" description="Dockside.2026.2160p.WEB-DL.DDP5.1.HDR.H.265-HARBOUR">
          {#each ['Title: Dockside', 'Year: 2026', 'Resolution: 2160p', 'Source: WEB-DL', 'Audio: DD+ 5.1', 'HDR: HDR10', 'Codec: H.265', 'Group: HARBOUR', 'Edition: none', 'Language: English'] as line (line)}
            <p>{line}</p>
          {/each}
          {#snippet actions()}<Button>Close</Button>{/snippet}
        </Dialog>
      </div>
    </Row>
  </ThemePair>
</Story>

<style>
  .form { display: grid; gap: var(--space-4); }
  .short { display: grid; max-height: 18rem; }
  .short :global(.dlg) { max-height: 18rem; }
</style>
