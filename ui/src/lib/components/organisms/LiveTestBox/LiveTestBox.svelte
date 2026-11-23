<!--
  LiveTestBox: paste a release title (or any input), see at once how corsarr reads it:
  the fields it recognised and which it did not (parse tester), and whether each condition
  matches (custom formats, quality), with an overall verdict and score.

  `test` runs on input after a short pause (or on the Test button with `live={false}`); stale
  answers are dropped, so only the result for the current input shows.
-->
<script lang="ts" module>
  export interface TestField { label: string; value?: string | null; mono?: boolean }
  export interface TestCondition {
    label: string;
    matched: boolean;
    /** What was compared: "matched '2160p' at 34", "no 'HDR' token". */
    detail?: string;
    required?: boolean;
    /** Score this condition adds when matched. */
    score?: number;
  }
  export interface TestResult {
    verdict?: { matched: boolean; text: string };
    score?: number;
    fields?: TestField[];
    conditions?: TestCondition[];
  }
</script>

<script lang="ts">
  import { untrack } from 'svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import Chip from '../../atoms/Chip/Chip.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import Spinner from '../../atoms/Spinner/Spinner.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import Textarea from '../../atoms/Textarea/Textarea.svelte';
  import PropertyList from '../../molecules/PropertyList/PropertyList.svelte';
  import { formatCount } from '../../../format/quantity';

  interface Props {
    /** Label of the input: "Release title". */
    label: string;
    value?: string;
    placeholder?: string;
    help?: string;
    /** Quick samples to fill the input. */
    samples?: string[];
    test: (input: string) => TestResult | Promise<TestResult>;
    /** Run while typing (after `delay` ms) or only on the Test button. */
    live?: boolean;
    delay?: number;
    rows?: number;
  }
  let {
    label, value = $bindable(''), placeholder, help, samples = [], test, live = true, delay = 250, rows = 2,
  }: Props = $props();

  const id = $props.id();
  let result = $state<TestResult>();
  let error = $state<string>();
  let running = $state(false);
  let seq = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;

  async function run(input: string) {
    clearTimeout(timer);
    const mine = ++seq;
    if (!input.trim()) { result = undefined; error = undefined; running = false; return; }
    running = true;
    try {
      const r = await test(input);
      if (mine !== seq) return;
      result = r;
      error = undefined;
    } catch (e) {
      if (mine !== seq) return;
      error = e instanceof Error ? e.message : String(e);
    } finally {
      if (mine === seq) running = false;
    }
  }
  function oninput() {
    if (!live) return;
    clearTimeout(timer);
    timer = setTimeout(() => run(value), delay);
  }
  function useSample(s: string) {
    value = s;
    run(s);
  }
  $effect(() => () => clearTimeout(timer));
  // A value given from outside (a sample in the URL) is tested once on mount.
  $effect(() => { untrack(() => { if (value) run(value); }); });

  const matchedCount = $derived(result?.conditions?.filter((c) => c.matched).length ?? 0);
  const unknown = $derived(result?.fields?.filter((f) => f.value === undefined || f.value === null || f.value === '').length ?? 0);
</script>

<div class="ltb">
  <div class="input">
    <label class="label" for="{id}-in">{label}</label>
    <Textarea id="{id}-in" bind:value {placeholder} {rows} mono spellcheck={false} aria-describedby={help ? `${id}-help` : undefined} {oninput}
      onkeydown={(e) => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); run(value); } }} />
    {#if help}<p class="help" id="{id}-help">{help}</p>{/if}
    {#if samples.length || !live}
      <div class="tools">
        {#if samples.length}
          <span class="samples-label">Try</span>
          {#each samples as s (s)}<button type="button" class="sample" onclick={() => useSample(s)} title={s}>{s}</button>{/each}
        {/if}
        {#if !live}<span class="push"><Button size="sm" variant="primary" icon="play" loading={running} disabled={!value.trim()} onclick={() => run(value)}>Test</Button></span>{/if}
      </div>
    {/if}
  </div>

  <section class="out" aria-labelledby="{id}-out" aria-busy={running}>
    <h4 class="visually-hidden" id="{id}-out">Result</h4>
    {#if error}
      <p class="error"><Icon name="alert-octagon" size="sm" /><span>Couldn't test this: {error}</span></p>
    {:else if !result}
      <p class="empty">{running ? 'Testing…' : 'Paste a title above to see how it is read.'}</p>
    {:else}
      <div class="body" class:stale={running}>
        <div class="verdict" role="status">
          {#if result.verdict}
            <StatusBadge label={result.verdict.text} icon={result.verdict.matched ? 'check-circle' : 'x-circle'} tone={result.verdict.matched ? 'ok' : 'error'} />
          {/if}
          {#if result.score !== undefined}<Chip variant="score" label="Score" score={result.score} />{/if}
          {#if result.conditions?.length}<span class="meta">{formatCount(matchedCount)} of {formatCount(result.conditions.length)} conditions match</span>{/if}
          {#if result.fields?.length}<span class="meta">{unknown ? `${formatCount(unknown)} not recognised` : 'Every field recognised'}</span>{/if}
          {#if running}<Spinner size="sm" label="Testing" />{/if}
        </div>

        {#if result.fields?.length}
          <PropertyList label="Fields read from the input" density="compact"
            items={result.fields.map((f, i) => ({ key: String(i), label: f.label, value: f.value, mono: f.mono }))} />
        {/if}

        {#if result.conditions?.length}
          <ul class="conds" aria-label="Conditions">
            {#each result.conditions as c, i (i)}
              <li class:miss={!c.matched}>
                <span class="mark" class:ok={c.matched}><Icon name={c.matched ? 'check-circle' : 'x-circle'} size="sm" /><span>{c.matched ? 'Matches' : 'No match'}</span></span>
                <span class="cl">
                  <span class="cname">{c.label}</span>
                  {#if c.required}<span class="req">Required</span>{/if}
                  {#if c.detail}<span class="cdetail">{c.detail}</span>{/if}
                </span>
                {#if c.score !== undefined}<Chip variant="score" label={c.label} score={c.matched ? c.score : 0} />{/if}
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    {/if}
  </section>
</div>

<style>
  .ltb { display: grid; gap: var(--space-4); min-width: 0; }
  .input { display: grid; gap: var(--space-1-5); }
  .label { font-size: var(--text-md); font-weight: var(--weight-medium); }
  .help { font-size: var(--text-sm); color: var(--ink-muted); }
  .tools { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-1-5); }
  .samples-label { font-size: var(--text-sm); color: var(--ink-muted); }
  .sample {
    font: inherit; font-family: var(--font-mono); font-size: var(--text-xs); color: var(--ink);
    max-width: 22rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    padding: 0 var(--space-2); height: var(--control-h-sm); border: var(--border-w) solid var(--line-strong);
    border-radius: var(--radius-sm); background: var(--surface-raised); cursor: pointer;
  }
  .sample:hover { background: var(--surface-hover); }
  .push { margin-left: auto; }
  .out { border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); background: var(--surface-raised); padding: var(--space-4); min-width: 0; }
  .empty { color: var(--ink-muted); font-size: var(--text-md); }
  .error { display: flex; gap: var(--space-1-5); align-items: flex-start; color: var(--tone-error-ink); font-size: var(--text-md); font-weight: var(--weight-medium); }
  .body { display: grid; gap: var(--space-4); transition: opacity var(--dur-fast) var(--ease-out); }
  .body.stale { opacity: 0.6; }
  .verdict { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2) var(--space-3); }
  .meta { font-size: var(--text-sm); color: var(--ink-muted); }
  .conds { list-style: none; padding: 0; margin: 0; display: grid; }
  .conds li { display: flex; align-items: center; gap: var(--space-3); padding: var(--space-2) 0; border-top: var(--border-w) solid var(--line); min-width: 0; }
  .conds li:first-child { border-top: 0; }
  .mark { display: inline-flex; align-items: center; gap: var(--space-1); width: 6.5rem; flex: none; font-size: var(--text-sm); font-weight: var(--weight-medium); color: var(--ink-muted); }
  .mark.ok { color: var(--tone-ok-ink); }
  .cl { display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--space-0-5) var(--space-2); flex: 1; min-width: 0; }
  .cname { font-size: var(--text-md); color: var(--ink); }
  .miss .cname { color: var(--ink-muted); }
  .req { font-size: var(--text-xs); color: var(--ink-subtle); }
  .cdetail { font-size: var(--text-xs); color: var(--ink-muted); font-family: var(--font-mono); overflow-wrap: anywhere; }
  @media (width < 48rem), (pointer: coarse) {
    .sample { height: var(--target-min); }
    .mark { width: auto; }
    .conds li { flex-wrap: wrap; }
  }
</style>
