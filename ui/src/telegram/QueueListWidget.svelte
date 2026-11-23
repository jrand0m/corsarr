<!--
  QueueListWidget: the Telegram page body for the queue list. One read-only QueueCard per download, each
  followed by a plain form posting `id` and `action` to `formAction` (pause or resume, replace,
  replace with lower resolution, kill). No scripts: every action is a named submit button.
  Replace, replace-lower and kill end in "…": the server answers with its confirmation.
-->
<script lang="ts">
  import QueueCard from '../lib/components/molecules/QueueCard/QueueCard.svelte';
  import Button from '../lib/components/atoms/Button/Button.svelte';
  import type { QueueCardData } from '../lib/components/molecules/QueueCard/QueueCard.svelte';
  import type { QueueListWidgetData, QueueWidgetAction } from './types';

  interface Props {
    data: QueueListWidgetData;
  }
  let { data }: Props = $props();

  const allowed = $derived(new Set<QueueWidgetAction>(data.actions ?? ['pause', 'resume', 'replace', 'replace-lower', 'kill']));
  const finished = (d: QueueCardData) => ['imported', 'removed', 'failed'].includes(d.state);
  const attention = $derived(data.downloads.filter((d) => d.rule || ['stalled', 'import-blocked', 'failed'].includes(d.state)).length);
  const summary = $derived(
    [`${data.downloads.length} ${data.downloads.length === 1 ? 'download' : 'downloads'}`, attention ? `${attention} need${attention === 1 ? 's' : ''} attention` : '']
      .filter(Boolean)
      .join(' · '),
  );
</script>

<div class="tg">
  <header class="head">
    <h2>{data.title ?? 'Queue'}</h2>
    <p class="summary num">{summary}</p>
  </header>

  {#if !data.downloads.length}
    <p class="empty">Nothing downloading. New grabs show up here.</p>
  {:else}
    <ul class="list">
      {#each data.downloads as d (d.id)}
        <li class="item">
          <QueueCard download={d} now={data.now} />
          {#if !finished(d)}
            <form class="actions" method="post" action={data.formAction} aria-label="Actions for {d.title}">
              <input type="hidden" name="id" value={d.id} />
              {#if d.paused && allowed.has('resume')}
                <Button type="submit" size="sm" icon="play" name="action" value="resume">Resume</Button>
              {:else if !d.paused && allowed.has('pause')}
                <Button type="submit" size="sm" icon="pause" name="action" value="pause">Pause</Button>
              {/if}
              {#if allowed.has('replace')}
                <Button type="submit" size="sm" variant="ghost" name="action" value="replace">Replace…</Button>
              {/if}
              {#if allowed.has('replace-lower')}
                <Button type="submit" size="sm" variant="ghost" name="action" value="replace-lower">Lower<span class="visually-hidden">{' resolution'}</span>…</Button>
              {/if}
              {#if allowed.has('kill')}
                <Button type="submit" size="sm" variant="ghost" icon="trash" name="action" value="kill" class="kill">Kill…</Button>
              {/if}
            </form>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .tg { display: grid; gap: var(--space-3); width: 100%; max-width: 30rem; margin-inline: auto; padding: var(--space-3); }
  .head { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: var(--space-1) var(--space-3); }
  h2 { font-size: var(--text-lg); }
  .summary { font-size: var(--text-sm); color: var(--ink-muted); }
  .empty { color: var(--ink-muted); }
  .list { display: grid; gap: var(--space-4); margin: 0; padding: 0; list-style: none; }
  .item { display: grid; gap: var(--space-1-5); min-width: 0; }
  .actions { display: flex; flex-wrap: wrap; gap: var(--space-1); justify-content: flex-end; }
  .actions :global(.kill) { color: var(--tone-error-ink); }
</style>
