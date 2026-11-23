<!--
  QueueCard: one download, compact: kind and title, what it covers, state with reason,
  progress, size, speed and ETA, the rule that will act on it and when, and its actions
  (Pause/Resume up front; prioritise, replace, replace with lower resolution and kill in a
  menu). Renders from one data object with no shell, drawer, store or event-stream
  dependency, so the same markup serves the phone queue, the dashboard and the Telegram HTML
  widget (which renders it without scripts: the action menu then stays closed).
  Kill, replace and replace-lower must be confirmed by the caller.
-->
<script lang="ts" module>
  import type { MediaKind } from '../../../tokens';
  import type { StatusValue } from '../../../status/vocabulary';

  export type QueueAction = 'pause' | 'resume' | 'top' | 'up' | 'down' | 'bottom' | 'replace' | 'replace-lower' | 'kill';

  export interface QueueCardData {
    id: string;
    title: string;
    kind: MediaKind;
    /** What it covers: "S02E03 · Customs", "S02 · 13 episodes". */
    subtitle?: string;
    state: StatusValue<'download'>;
    /** Qualifier after the state: "no peers for 2h", "sample only". */
    reason?: string;
    paused?: boolean;
    held?: boolean;
    shadow?: boolean;
    /** 0–1; omit for unknown (metadata pending). */
    progress?: number;
    size?: number;
    downloaded?: number;
    /** Bytes per second. */
    speed?: number;
    /** Seconds remaining. */
    eta?: number;
    rule?: { badge: StatusValue<'ruleBadge'>; at: Date | string | number; action: string; dryRun?: boolean };
    /** Detail drawer link. */
    href?: string;
  }
</script>

<script lang="ts">
  import Artwork from '../../atoms/Artwork/Artwork.svelte';
  import MediaKindIcon from '../../atoms/MediaKindIcon/MediaKindIcon.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import Progress from '../../atoms/Progress/Progress.svelte';
  import Countdown from '../../atoms/Countdown/Countdown.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import ActionMenu, { type ActionItem } from '../ActionMenu/ActionMenu.svelte';
  import { formatBytes, formatSpeed, formatDuration } from '../../../format/quantity';
  import type { Tone } from '../../../tokens';

  interface Props {
    download: QueueCardData;
    /** Actions offered; default all that fit the state. */
    actions?: QueueAction[];
    onaction?: (action: QueueAction, id: string) => void;
    /** Poster thumbnail URL (cached image); the kind icon stands in when absent. */
    art?: string | null;
    /** Fixed "now" (ms) for stories, tests and server-rendered widgets. */
    now?: number;
  }
  let { download: d, actions, onaction, art, now }: Props = $props();

  const finished = $derived(['imported', 'removed', 'failed'].includes(d.state));
  const allowed = $derived(new Set<QueueAction>(actions ?? ['pause', 'resume', 'top', 'up', 'down', 'bottom', 'replace', 'replace-lower', 'kill']));
  const toneFor: Partial<Record<string, Tone>> = { stalled: 'warn', failed: 'error', 'import-blocked': 'error', completed: 'ok', imported: 'ok' };
  const barTone = $derived(d.paused || d.held ? 'neutral' : (toneFor[d.state] ?? 'info'));
  const menu = $derived<ActionItem[]>(
    ([
      { id: 'top', label: 'Move to top', icon: 'chevron-up', group: 'order', hint: 'Shift+Home' },
      { id: 'up', label: 'Move up', icon: 'arrow-up', group: 'order', hint: 'Shift+↑' },
      { id: 'down', label: 'Move down', icon: 'arrow-down', group: 'order', hint: 'Shift+↓' },
      { id: 'bottom', label: 'Move to bottom', icon: 'chevron-down', group: 'order', hint: 'Shift+End' },
      { id: 'replace', label: 'Replace with another release…', icon: 'swap', group: 'replace', hint: 'R' },
      { id: 'replace-lower', label: 'Replace with lower resolution…', icon: 'lower', group: 'replace', hint: 'L' },
      { id: 'kill', label: 'Kill download…', icon: 'trash', group: 'kill', danger: true, hint: 'Del' },
    ] satisfies ActionItem[]).filter((a) => allowed.has(a.id as QueueAction)),
  );
  const facts = $derived(
    [
      d.size !== undefined ? (d.downloaded !== undefined && d.downloaded < d.size ? `${formatBytes(d.downloaded)} of ${formatBytes(d.size)}` : formatBytes(d.size)) : undefined,
      d.speed && !d.paused ? formatSpeed(d.speed) : undefined,
      d.eta !== undefined && !d.paused && d.state === 'downloading' ? `${formatDuration(d.eta)} left` : undefined,
    ].filter(Boolean) as string[],
  );
  const act = (a: QueueAction) => onaction?.(a, d.id);
</script>

<article class="qc" aria-labelledby="qc-{d.id}">
  <div class="top">
    {#if art !== undefined}
      <Artwork src={art} title={d.title} kind={d.kind} width={40} />
    {:else}
      <span class="kind"><MediaKindIcon kind={d.kind} /></span>
    {/if}
    <div class="names">
      <h3 class="title" id="qc-{d.id}">{#if d.href}<a href={d.href}>{d.title}</a>{:else}{d.title}{/if}</h3>
      {#if d.subtitle}<p class="sub">{d.subtitle}</p>{/if}
    </div>
  </div>

  <div class="badges">
    <StatusBadge family="download" value={d.state} detail={d.reason} size="sm" />
    {#if d.paused}<StatusBadge family="downloadModifier" value="paused" size="sm" />{/if}
    {#if d.held}<StatusBadge family="downloadModifier" value="held" size="sm" />{/if}
    {#if d.shadow}<StatusBadge family="downloadModifier" value="shadow" size="sm" />{/if}
  </div>

  {#if !finished && (d.progress !== undefined || d.state === 'downloading')}
    <div class="progress">
      <Progress label="Progress of {d.title}" value={d.progress} tone={barTone} size="sm" />
      <span class="facts num">
        {#if d.progress !== undefined}<span class="pct">{Math.floor(d.progress * 100)}%</span>{/if}
        {#each facts as f (f)}<span>{f}</span>{/each}
      </span>
    </div>
  {:else if !finished && facts.length}
    <span class="facts num">{#each facts as f (f)}<span>{f}</span>{/each}</span>
  {/if}

  {#if d.rule}
    <p class="rule">
      <StatusBadge family="ruleBadge" value={d.rule.badge} size="sm" appearance="plain" />
      <span class="when">
        {#if d.rule.dryRun}<span class="dry">Dry run:</span>{/if}
        <Countdown to={d.rule.at} action={d.rule.action} tone="warn" {now} expiredLabel="acting now" />
      </span>
    </p>
  {/if}

  {#if onaction && !finished}
    <div class="actions">
      {#if d.paused && allowed.has('resume')}
        <Button size="sm" icon="play" onclick={() => act('resume')}>Resume</Button>
      {:else if !d.paused && allowed.has('pause')}
        <Button size="sm" icon="pause" onclick={() => act('pause')}>Pause</Button>
      {/if}
      {#if menu.length}
        <ActionMenu size="sm" items={menu} label="More actions for {d.title}" onselect={(id) => act(id as QueueAction)} />
      {/if}
    </div>
  {/if}
</article>

<style>
  .qc {
    display: grid;
    gap: var(--space-2);
    min-width: 0;
    padding: var(--space-3);
    color: var(--ink);
    background: var(--surface-raised);
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-lg);
  }
  .top { display: flex; gap: var(--space-2); align-items: flex-start; min-width: 0; }
  .kind { display: grid; place-items: center; width: var(--control-h-sm); height: var(--control-h-sm); flex: none; background: var(--surface-sunken); border-radius: var(--radius-md); }
  .names { display: grid; gap: var(--space-0-5); min-width: 0; }
  .title {
    font-size: var(--text-md);
    font-weight: var(--weight-semibold);
    line-height: var(--leading-snug);
    overflow-wrap: anywhere;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .title a { color: inherit; text-decoration: none; }
  .title a:hover { text-decoration: underline; }
  .sub { font-size: var(--text-sm); color: var(--ink-muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .badges { display: flex; flex-wrap: wrap; gap: var(--space-1); }
  .progress { display: grid; gap: var(--space-1); }
  .facts { display: flex; flex-wrap: wrap; gap: var(--space-0-5) var(--space-3); font-size: var(--text-xs); color: var(--ink-muted); }
  .pct { color: var(--ink); font-weight: var(--weight-medium); }
  .rule { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-1) var(--space-2); font-size: var(--text-sm); max-width: none; }
  .when { display: inline-flex; align-items: center; gap: var(--space-1); color: var(--ink-muted); }
  .dry { color: var(--tone-info-ink); font-weight: var(--weight-medium); }
  .actions { display: flex; gap: var(--space-1); justify-content: flex-end; }
</style>
