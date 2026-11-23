<!--
  WizardPage (template): a guided flow on its own page (add an object, run a migration). Page
  title, a back link to where the flow started, an optional banner (dry run, resumed), the Wizard
  stepper in a readable column, and an optional help column beside it on wide screens.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import Link from '../../atoms/Link/Link.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import Wizard, { type WizardStep } from '../../organisms/Wizard/Wizard.svelte';

  interface Props {
    title: string;
    description?: string;
    back?: { href: string; label: string };
    banner?: Snippet;
    steps: WizardStep[];
    current?: string;
    completed?: string[];
    validate?: (step: string) => string[] | Promise<string[]>;
    resumed?: boolean;
    busy?: boolean;
    finishLabel?: string;
    onstep?: (step: string) => void;
    onfinish?: () => void;
    oncancel?: () => void;
    /** Body of the current step. */
    children: Snippet<[WizardStep]>;
    /** Help beside the steps: what happens next, links to docs. */
    aside?: Snippet;
  }
  let {
    title, description, back, banner, steps, current = $bindable(steps[0]?.id), completed = $bindable([]), validate, resumed = false,
    busy = false, finishLabel, onstep, onfinish, oncancel, children: body, aside,
  }: Props = $props();
</script>

<div class="frame">
  <div class="wp" class:with-aside={!!aside}>
    <header class="head">
      {#if back}<Link href={back.href} variant="quiet"><Icon name="arrow-left" size="sm" />{back.label}</Link>{/if}
      <h1>{title}</h1>
      {#if description}<p class="desc">{description}</p>{/if}
    </header>
    {#if banner}<div class="banner">{@render banner()}</div>{/if}
    <div class="flow">
      <Wizard {steps} bind:current bind:completed {validate} {resumed} {busy} {finishLabel} {onstep} {onfinish} {oncancel} label="{title} steps">
        {#snippet children(step)}{@render body(step)}{/snippet}
      </Wizard>
    </div>
    {#if aside}<aside class="aside" aria-label="Help">{@render aside()}</aside>{/if}
  </div>
</div>

<style>
  .frame { container-type: inline-size; min-width: 0; }
  .wp { display: grid; gap: var(--space-5) var(--space-8); grid-template-columns: minmax(0, 1fr); max-width: 72rem; }
  .head { display: grid; gap: var(--space-2); justify-items: start; }
  h1 { font-size: var(--text-xl); line-height: var(--leading-tight); }
  .desc { color: var(--ink-muted); max-width: var(--measure); }
  .flow { min-width: 0; max-width: 48rem; }
  .aside { display: grid; gap: var(--space-3); align-content: start; font-size: var(--text-md); color: var(--ink-muted); max-width: var(--measure); }
  @container (width >= 60rem) {
    .with-aside { grid-template-columns: minmax(0, 48rem) minmax(14rem, 1fr); }
    .head, .banner { grid-column: 1 / -1; }
  }
</style>
