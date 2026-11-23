<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ConfirmDialog, { type ConfirmChoice } from './ConfirmDialog.svelte';
  import ToastRegion from '../ToastRegion/ToastRegion.svelte';
  import Checkbox from '../../atoms/Checkbox/Checkbox.svelte';
  import QueueCard, { type QueueCardData, type QueueAction } from '../../molecules/QueueCard/QueueCard.svelte';
  import { ToastQueue } from '../../../toast/toasts.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const killChoices: ConfirmChoice[] = [
    {
      value: 'research', label: 'Blocklist and search again', description: 'Best when the release itself is bad.',
      consequences: ['Blocklists this release so it is never grabbed again', 'Searches for another release now'],
    },
    { value: 'remove', label: 'Just remove', description: 'The episode goes back to Wanted.' },
  ];
  const now = Date.parse('2026-11-22T14:05:00Z');
  const stalled: QueueCardData = {
    id: 'd2', title: 'The Long Haul', kind: 'tv', subtitle: 'S02E03 · Customs', state: 'stalled', reason: 'no peers for 2h',
    progress: 0.81, size: 4.7e9, downloaded: 3.8e9, speed: 0,
  };

  const { Story } = defineMeta({
    title: 'Organisms/ConfirmDialog',
    component: ConfirmDialog,
    tags: ['autodocs'],
    args: {
      title: 'Kill The Long Haul S02E03?',
      consequences: ['Removes the download from qBittorrent'],
      choices: killChoices,
      confirmLabel: 'Kill download',
      undoable: true,
    },
  });
</script>

<script lang="ts">
  const toasts = new ToastQueue();
  let open = $state(false);
  let deleteFiles = $state(false);
  let gone = $state(false);

  function onaction(a: QueueAction) {
    if (a === 'kill') open = true;
  }
  function killed(choice: string | undefined) {
    gone = true;
    toasts.push({
      message: choice === 'research' ? 'Killed The Long Haul S02E03 and blocklisted the release' : 'Killed The Long Haul S02E03',
      detail: choice === 'research' ? 'Searching for another release.' : 'S02E03 is back in Wanted.',
      onundo: () => (gone = false),
    });
  }
</script>

<Story name="Playground">
  {#snippet template(args)}<ConfirmDialog {...args} modal={false} open />{/snippet}
</Story>

<Story name="Kill flow (card, confirm, toast with undo)" asChild>
  <div class="flow">
    {#if !gone}
      <div class="card"><QueueCard download={stalled} {now} actions={['pause', 'kill']} {onaction} /></div>
    {:else}
      <p class="gone">The Long Haul S02E03 was killed. Use Undo in the notification to bring it back.</p>
    {/if}
    <ConfirmDialog
      bind:open title="Kill The Long Haul S02E03?" consequences={['Removes the download from qBittorrent']}
      choices={killChoices} confirmLabel="Kill download" onconfirm={killed}
    >
      <Checkbox bind:checked={deleteFiles} label="Also delete downloaded files" description="3.8 GB in /downloads/tv-corsarr" />
    </ConfirmDialog>
    <ToastRegion queue={toasts} />
  </div>
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Kill: consequences follow the choice">
      <ConfirmDialog modal={false} open title="Kill The Long Haul S02E03?" consequences={['Removes the download from qBittorrent']} choices={killChoices} confirmLabel="Kill download">
        <Checkbox label="Also delete downloaded files" description="3.8 GB in /downloads/tv-corsarr" />
      </ConfirmDialog>
    </Row>
    <Row label="Permanent (no undo)">
      <ConfirmDialog
        modal={false} open title="Purge 3 files from the recycle bin?" undoable={false} confirmLabel="Purge now"
        consequences={['Deletes 3 files (12.4 GB) from disk', 'Frees the space immediately']}
      />
    </Row>
    <Row label="Weighty but safe (primary), custom undo statement">
      <ConfirmDialog
        modal={false} open title="Switch TV from shadow to live?" variant="primary" confirmLabel="Go live"
        undoable="Switch back to shadow at any time in Settings › Media kinds."
        consequences={['corsarr starts grabbing and importing TV itself', 'Sonarr-era downloads already in the queue are adopted']}
      />
    </Row>
    <Row label="Running (cannot be dismissed)">
      <ConfirmDialog modal={false} open loading title="Mark Dockside (2026) as failed?" confirmLabel="Mark failed" consequences={['Blocklists the release', 'Searches again']} />
    </Row>
  </ThemePair>
</Story>

<style>
  .flow { display: grid; gap: var(--space-4); min-height: 16rem; }
  .card { width: min(100%, 22rem); }
  .gone { color: var(--ink-muted); }
</style>
