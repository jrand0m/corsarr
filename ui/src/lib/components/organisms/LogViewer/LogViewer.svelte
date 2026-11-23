<!--
  LogViewer (lazy chunk): the live tail of corsarr's log. The page appends lines from
  the SSE stream; the viewer filters by level, module and text, follows the end while you read
  the newest lines, pauses when you scroll up (and says how many lines arrived since), and
  downloads what the filters show. Virtualised: 50 000 lines keep a viewport of DOM. Click a
  line to read it whole with its fields. Load it with `loadLogViewer()`.
-->
<script lang="ts">
  import { tick, untrack } from 'svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import Select from '../../atoms/Select/Select.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import TextInput from '../../atoms/TextInput/TextInput.svelte';
  import IconButton from '../../atoms/IconButton/IconButton.svelte';
  import EmptyState from '../../molecules/EmptyState/EmptyState.svelte';
  import PropertyList from '../../molecules/PropertyList/PropertyList.svelte';
  import { formatAbsolute, toDate } from '../../../format/time';
  import { formatCount } from '../../../format/quantity';
  import { formatFields, formatLogLine, formatLogTime, matchesLog, type LogLevel, type LogLine } from '../../../format/log';

  interface Props {
    lines: LogLine[];
    label?: string;
    /** Stream state, shown in the toolbar. */
    connection?: 'live' | 'reconnecting' | 'offline';
    following?: boolean;
    minLevel?: LogLevel;
    module?: string;
    text?: string;
    height?: string;
    filename?: string;
    /** Replaces the in-browser download (receives the file text). */
    ondownload?: (text: string, filename: string) => void;
    onfilterchange?: (f: { minLevel: LogLevel; module: string; text: string }) => void;
  }
  let {
    lines, label = 'Log', connection, following = $bindable(true), minLevel = $bindable('info'), module = $bindable(''),
    text = $bindable(''), height = '32rem', filename = 'corsarr.log', ondownload, onfilterchange,
  }: Props = $props();

  const OVERSCAN = 10;
  let scroller = $state<HTMLElement>();
  let scrollTop = $state(0);
  let viewH = $state(0);
  let rowH = $state(22);
  let pausedAt = $state(0);
  let selected = $state<number>();

  const filtered = $derived(lines.filter((l) => matchesLog(l, { minLevel, module, text })));
  const modules = $derived([...new Set(lines.map((l) => l.module))].sort());
  const first = $derived(Math.max(0, Math.floor(scrollTop / rowH) - OVERSCAN));
  const last = $derived(Math.min(filtered.length, Math.ceil((scrollTop + (viewH || 600)) / rowH) + OVERSCAN));
  const visible = $derived(filtered.slice(first, last));
  const newSince = $derived(following ? 0 : Math.max(0, filtered.length - pausedAt));
  const current = $derived(selected === undefined ? undefined : lines.find((l) => l.seq === selected));
  const levelTone: Record<LogLevel, 'neutral' | 'info' | 'warn' | 'error'> = { trace: 'neutral', debug: 'neutral', info: 'info', warn: 'warn', error: 'error' };

  $effect(() => {
    const h = scroller?.querySelector<HTMLElement>('.line')?.offsetHeight;
    if (h && h !== rowH) rowH = h;
  });

  // Follow the end: after new lines (or a filter change) land, scroll to the bottom.
  let programmatic = false;
  $effect(() => {
    void filtered.length;
    if (!following || !scroller) return;
    tick().then(() => {
      if (!scroller) return;
      programmatic = true;
      scroller.scrollTop = scroller.scrollHeight;
      scrollTop = scroller.scrollTop;
    });
  });
  function onscroll() {
    if (!scroller) return;
    scrollTop = scroller.scrollTop;
    if (programmatic) { programmatic = false; return; }
    const atEnd = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < rowH;
    if (following && !atEnd) pause();
    else if (!following && atEnd) following = true;
  }
  function pause() {
    following = false;
  }
  // Remember where reading stopped, however following was turned off (button, scroll, prop).
  $effect(() => {
    if (!following) pausedAt = untrack(() => filtered.length);
  });
  function follow() {
    following = true;
  }
  function filterChanged() {
    onfilterchange?.({ minLevel, module, text });
  }
  function download() {
    const body = filtered.map(formatLogLine).join('\n') + '\n';
    if (ondownload) return ondownload(body, filename);
    const url = URL.createObjectURL(new Blob([body], { type: 'text/plain' }));
    const a = Object.assign(document.createElement('a'), { href: url, download: filename });
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  function parts(s: string): { t: string; hit: boolean }[] {
    const q = text.trim();
    if (!q) return [{ t: s, hit: false }];
    const out: { t: string; hit: boolean }[] = [];
    const low = s.toLowerCase(), ql = q.toLowerCase();
    let i = 0;
    for (let j = low.indexOf(ql); j !== -1; j = low.indexOf(ql, i)) {
      if (j > i) out.push({ t: s.slice(i, j), hit: false });
      out.push({ t: s.slice(j, j + q.length), hit: true });
      i = j + q.length;
    }
    if (i < s.length) out.push({ t: s.slice(i), hit: false });
    return out;
  }
</script>

<section class="lv" aria-label={label}>
  <div class="toolbar">
    <span class="filter">
      <Select size="sm" aria-label="Lowest level shown" bind:value={minLevel} onchange={filterChanged} options={[
        { value: 'trace', label: 'Trace and up' }, { value: 'debug', label: 'Debug and up' }, { value: 'info', label: 'Info and up' },
        { value: 'warn', label: 'Warnings and errors' }, { value: 'error', label: 'Errors only' },
      ]} />
    </span>
    <span class="filter">
      <Select size="sm" aria-label="Module" bind:value={module} onchange={filterChanged} options={[{ value: '', label: 'All modules' }, ...modules.map((m) => ({ value: m, label: m }))]} />
    </span>
    <span class="search">
      <TextInput size="sm" type="search" icon="search" aria-label="Filter log text" placeholder="Filter text" bind:value={text} oninput={filterChanged} />
    </span>
    <span class="push"></span>
    {#if connection}<StatusBadge family="connection" value={connection} size="sm" />{/if}
    {#if following}
      <Button size="sm" icon="pause" onclick={pause}>Pause</Button>
    {:else}
      <Button size="sm" icon="play" onclick={follow}>Follow</Button>
    {/if}
    <IconButton size="sm" icon="download" label="Download shown lines" variant="secondary" onclick={download} disabled={!filtered.length} />
  </div>

  <p class="count">
    <span class="num">{filtered.length === lines.length ? `${formatCount(lines.length)} lines` : `${formatCount(filtered.length)} of ${formatCount(lines.length)} lines`}</span>
    {#if !following}<span class="paused"><Icon name="pause-circle" size="sm" />Paused</span>{/if}
  </p>

  <div class="frame">
    {#if !filtered.length}
      <div class="empty" style:height>
        <EmptyState title={lines.length ? 'No lines match these filters' : 'No log lines yet'} icon="list" size="sm">
          {#if lines.length}Lower the level or clear the text filter.{/if}
        </EmptyState>
      </div>
    {:else}
      <div class="viewport" style:height bind:this={scroller} bind:clientHeight={viewH} {onscroll} role="log" aria-label="{label} lines" aria-live="off" tabindex="-1">
        <ol class="lines" style:padding-top="{first * rowH}px" style:padding-bottom="{Math.max(0, filtered.length - last) * rowH}px">
          {#each visible as l (l.seq)}
            <li class="line lvl-{l.level}" class:sel={selected === l.seq}>
              <button type="button" class="lb" aria-pressed={selected === l.seq} onclick={() => (selected = selected === l.seq ? undefined : l.seq)} title={formatLogLine(l)}>
                <time class="t" datetime={toDate(l.at).toISOString()}>{formatLogTime(l.at)}</time>
                <span class="lvl">{#if l.level === 'error'}<Icon name="alert-octagon" size="sm" />{:else if l.level === 'warn'}<Icon name="alert-triangle" size="sm" />{/if}{l.level}</span>
                <span class="mod">{l.module}</span>
                <span class="msg">{#each parts(l.message) as p, i (i)}{#if p.hit}<mark>{p.t}</mark>{:else}{p.t}{/if}{/each}{#if l.fields}<span class="fields">{` ${formatFields(l.fields)}`}</span>{/if}</span>
              </button>
            </li>
          {/each}
        </ol>
      </div>
      {#if newSince > 0}
        <div class="jump"><Button size="sm" variant="primary" icon="arrow-down" onclick={follow}>{newSince === 1 ? '1 new line' : `${formatCount(newSince)} new lines`}</Button></div>
      {/if}
    {/if}
  </div>

  {#if current}
    <section class="detail" aria-label="Selected line">
      <div class="dhead">
        <StatusBadge label={current.level} tone={levelTone[current.level]} icon={current.level === 'error' ? 'alert-octagon' : current.level === 'warn' ? 'alert-triangle' : 'info'} size="sm" />
        <span class="dmeta">{formatAbsolute(toDate(current.at))} · {current.module}</span>
        <IconButton size="sm" icon="x" label="Close line details" onclick={() => (selected = undefined)} />
      </div>
      <p class="dmsg">{current.message}</p>
      {#if current.fields}
        <PropertyList density="compact" items={Object.entries(current.fields).map(([k, v]) => ({ key: k, label: k, value: String(v), mono: true, copy: true }))} />
      {/if}
    </section>
  {/if}
</section>

<style>
  .lv { display: grid; gap: var(--space-2); min-width: 0; }
  .toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); }
  .search { flex: 1 1 12rem; max-width: 22rem; }
  .push { flex: 1; }
  .count { display: flex; gap: var(--space-3); align-items: center; font-size: var(--text-sm); color: var(--ink-muted); }
  .paused { display: inline-flex; align-items: center; gap: var(--space-1); color: var(--tone-warn-ink); font-weight: var(--weight-medium); }
  .frame { position: relative; min-width: 0; }
  .viewport, .empty {
    overflow: auto; overscroll-behavior: contain; border: var(--border-w) solid var(--line); border-radius: var(--radius-lg);
    background: var(--surface-sunken); scrollbar-color: var(--line-strong) transparent;
  }
  .empty { display: grid; place-items: center; }
  .lines { list-style: none; margin: 0; padding-left: 0; padding-right: 0; min-width: max-content; }
  .line { height: 1.375rem; }
  .lb {
    display: flex; align-items: center; gap: var(--space-3); width: 100%; height: 100%; padding: 0 var(--space-3);
    font-family: var(--font-mono); font-size: var(--text-xs); line-height: 1.375rem; color: var(--ink);
    background: none; border: 0; text-align: start; cursor: pointer; white-space: pre;
  }
  .lb:hover { background: var(--surface-hover); }
  .sel .lb { background: var(--surface-selected); }
  .t { color: var(--ink-subtle); flex: none; }
  .lvl { display: inline-flex; align-items: center; gap: var(--space-1); width: 4.25rem; flex: none; text-transform: uppercase; color: var(--ink-muted); }
  .lvl-info .lvl { color: var(--tone-info-ink); }
  .lvl-warn .lvl { color: var(--tone-warn-ink); font-weight: var(--weight-semibold); }
  .lvl-error .lvl { color: var(--tone-error-ink); font-weight: var(--weight-semibold); }
  .lvl-trace .lb, .lvl-debug .lb { color: var(--ink-muted); }
  .mod { color: var(--accent-ink); flex: none; min-width: 7rem; }
  .fields { color: var(--ink-muted); }
  mark { background: var(--tone-warn-tint); color: var(--ink); border-radius: var(--radius-xs); outline: var(--border-w) solid var(--tone-warn-line); }
  .jump { position: absolute; bottom: var(--space-3); left: 50%; transform: translateX(-50%); box-shadow: var(--shadow-md); border-radius: var(--radius-md); }
  .detail { display: grid; gap: var(--space-2); padding: var(--space-3) var(--space-4); border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); background: var(--surface-raised); }
  .dhead { display: flex; align-items: center; gap: var(--space-2); }
  .dmeta { font-size: var(--text-sm); color: var(--ink-muted); flex: 1; }
  .dmsg { font-family: var(--font-mono); font-size: var(--text-sm); white-space: pre-wrap; overflow-wrap: anywhere; }
  @media (width < 48rem), (pointer: coarse) {
    .search { max-width: none; flex-basis: 100%; order: 5; }
    .line { height: var(--target-min); }
    .lb { line-height: var(--target-min); }
  }
</style>
