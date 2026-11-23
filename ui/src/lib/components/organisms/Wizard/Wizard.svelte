<!--
  Wizard: a multi-step flow (add an indexer from a plugin, the migration run). Shows
  every step with its state, lets the owner go back to any finished step, validates the
  current step before moving on (errors listed above the form and focused), and resumes:
  pass `completed` and `current` from the saved run and it opens where they left off. Focus
  moves to the step heading on every change, so screen readers hear the new step. The
  route owns URL sync: bind `current` to `/system/migration/runs/{id}/{step}`.
-->
<script lang="ts" module>
  export interface WizardStep {
    id: string;
    label: string;
    /** One line under the step heading. */
    description?: string;
    optional?: boolean;
  }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import Progress from '../../atoms/Progress/Progress.svelte';
  import Banner from '../../molecules/Banner/Banner.svelte';

  interface Props {
    steps: WizardStep[];
    current?: string;
    completed?: string[];
    /** Problems that block leaving the step; empty when it is fine. */
    validate?: (step: string) => string[] | Promise<string[]>;
    /** The body of the current step. */
    children: Snippet<[WizardStep]>;
    /** Opened from a saved run: shows where it picked up. */
    resumed?: boolean;
    finishLabel?: string;
    nextLabel?: string;
    busy?: boolean;
    onstep?: (step: string) => void;
    onfinish?: () => void;
    oncancel?: () => void;
    label?: string;
  }
  let {
    steps, current = $bindable(steps[0]?.id), completed = $bindable([]), validate, children, resumed = false,
    finishLabel = 'Finish', nextLabel = 'Continue', busy = false, onstep, onfinish, oncancel, label = 'Steps',
  }: Props = $props();

  const id = $props.id();
  const index = $derived(Math.max(0, steps.findIndex((s) => s.id === current)));
  const step = $derived(steps[index]);
  const last = $derived(index === steps.length - 1);
  let errors = $state<string[]>([]);
  let checking = $state(false);
  let heading = $state<HTMLElement>();
  let summary = $state<HTMLElement>();
  let moved = false;

  const done = (s: WizardStep) => completed.includes(s.id);
  /** A step is reachable when every step before it is finished (or it is the current one). */
  const reachable = (i: number) => i === index || steps.slice(0, i).every((s) => done(s) || s.optional);

  $effect(() => {
    void current;
    if (moved) heading?.focus();
  });

  function go(target: string) {
    errors = [];
    moved = true;
    current = target;
    onstep?.(target);
  }
  async function next() {
    checking = true;
    const problems = (await validate?.(step.id)) ?? [];
    checking = false;
    if (problems.length) {
      errors = problems;
      queueMicrotask(() => summary?.focus());
      return;
    }
    if (!done(step)) completed = [...completed, step.id];
    if (last) onfinish?.();
    else go(steps[index + 1].id);
  }
</script>

<div class="wizard">
  <nav aria-label={label}>
    <p class="count">Step {index + 1} of {steps.length}<span class="sep">·</span><span>{step.label}</span></p>
    <div class="bar"><Progress label="Steps finished" value={completed.length / steps.length} size="sm" /></div>
    <ol>
      {#each steps as s, i (s.id)}
        {@const state = i === index ? 'current' : done(s) ? 'done' : 'todo'}
        <li class={state}>
          {#snippet marker()}
            <span class="mark" aria-hidden="true">{#if state === 'done'}<Icon name="check" size="sm" />{:else}{i + 1}{/if}</span>
            <span class="name">
              <span class="label">{s.label}</span>
              {#if s.optional}<span class="opt">Optional</span>{/if}
            </span>
            {#if state === 'done'}<span class="visually-hidden">(finished)</span>{/if}
          {/snippet}
          {#if i !== index && reachable(i)}
            <button type="button" onclick={() => go(s.id)}>{@render marker()}</button>
          {:else}
            <span class="step" aria-current={i === index ? 'step' : undefined} aria-disabled={i === index ? undefined : 'true'}>{@render marker()}</span>
          {/if}
        </li>
      {/each}
    </ol>
  </nav>

  <section class="body" aria-labelledby="{id}-h">
    {#if resumed && index > 0}
      <p class="resumed"><Icon name="refresh" size="sm" />Picked up where you left off. Earlier steps are kept.</p>
    {/if}
    <header>
      <h2 id="{id}-h" tabindex="-1" bind:this={heading}>{step.label}</h2>
      {#if step.description}<p class="desc">{step.description}</p>{/if}
    </header>
    {#if errors.length}
      <div class="errors" role="alert" tabindex="-1" bind:this={summary}>
        <Banner tone="error" icon="alert-octagon" title={errors.length === 1 ? 'Fix this to continue' : `Fix these ${errors.length} things to continue`}>
          <ul>{#each errors as e, i (i)}<li>{e}</li>{/each}</ul>
        </Banner>
      </div>
    {/if}
    <div class="content">{#key step.id}{@render children(step)}{/key}</div>
  </section>

  <footer>
    {#if oncancel}<Button variant="ghost" onclick={oncancel}>Cancel</Button>{/if}
    <span class="grow"></span>
    {#if index > 0}<Button icon="chevron-left" onclick={() => go(steps[index - 1].id)}>Back</Button>{/if}
    <Button variant="primary" iconEnd={last ? undefined : 'chevron-right'} loading={checking || busy} onclick={next}>
      {last ? finishLabel : nextLabel}
    </Button>
  </footer>
</div>

<style>
  .wizard {
    display: grid;
    grid-template-columns: minmax(11rem, 14rem) minmax(0, 1fr);
    grid-template-areas: 'nav body' 'nav foot';
    gap: var(--space-4) var(--space-8);
    color: var(--ink);
  }
  nav { grid-area: nav; }
  .count, .bar { display: none; }
  ol { display: grid; gap: var(--space-0-5); padding: 0; list-style: none; }
  button, .step {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    width: 100%;
    min-height: var(--control-h-lg);
    padding: var(--space-1) var(--space-2);
    font-size: var(--text-md);
    text-align: left;
    color: var(--ink-muted);
    background: none;
    border: 0;
    border-radius: var(--radius-md);
  }
  button:hover { background: var(--surface-hover); color: var(--ink); }
  .mark {
    display: grid;
    place-items: center;
    flex: none;
    width: 1.5rem;
    height: 1.5rem;
    font-size: var(--text-xs);
    font-weight: var(--weight-semibold);
    font-variant-numeric: tabular-nums;
    border: var(--border-w) solid var(--line-strong);
    border-radius: var(--radius-round);
  }
  .name { display: grid; min-width: 0; line-height: var(--leading-snug); }
  .opt { font-size: var(--text-xs); color: var(--ink-subtle); }
  .done .mark { color: var(--tone-ok-ink); background: var(--tone-ok-tint); border-color: var(--tone-ok-line); }
  .current .step { color: var(--ink); background: var(--surface-selected); }
  .current .label { font-weight: var(--weight-semibold); }
  .current .mark { color: var(--ink-on-accent); background: var(--accent); border-color: var(--accent); }
  .todo .step { color: var(--ink-subtle); }

  .body { grid-area: body; display: grid; gap: var(--space-4); align-content: start; min-width: 0; }
  header { display: grid; gap: var(--space-1); }
  h2 { font-size: var(--text-xl); }
  .desc { color: var(--ink-muted); }
  .resumed { display: flex; align-items: center; gap: var(--space-1-5); font-size: var(--text-sm); color: var(--tone-info-ink); }
  .errors ul { margin: var(--space-1) 0 0; padding-left: var(--space-4); }
  .errors:focus-visible { outline-offset: var(--space-0-5); border-radius: var(--radius-lg); }
  .content { min-width: 0; }

  footer {
    grid-area: foot;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    padding-top: var(--space-4);
    border-top: var(--border-w) solid var(--line);
  }
  .grow { flex: 1; }

  @media (width < 48rem) {
    .wizard { grid-template-columns: minmax(0, 1fr); grid-template-areas: 'nav' 'body' 'foot'; gap: var(--space-4); }
    ol { display: none; }
    .count { display: flex; gap: var(--space-1); font-size: var(--text-sm); color: var(--ink-muted); }
    .bar { display: block; margin-top: var(--space-1-5); }
    .sep { color: var(--ink-subtle); }
    footer { position: sticky; bottom: 0; padding-bottom: var(--space-2); background: var(--surface-ground); }
  }
</style>
