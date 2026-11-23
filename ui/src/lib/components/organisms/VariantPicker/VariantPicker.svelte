<!--
  VariantPicker: pick one release from ranked alternatives (replace a download, replace
  with lower resolution, item-mode search). One candidate is the default and is picked
  automatically when the visible countdown ends; choosing another stops the countdown;
  Cancel keeps things as they are. Keyboard: arrows move between candidates, Enter picks,
  Escape cancels. Compact and self-contained: renders from props only (no shell, store or
  event stream) and is a plain form of radios and submit buttons, so the same markup works
  as the Telegram HTML widget template without scripts (`formAction` receives the post).
-->
<script lang="ts" module>
  export interface Variant {
    id: string;
    /** Release name as published. */
    title: string;
    /** "WEB-DL 1080p". */
    quality: string;
    /** Bytes. */
    size: number;
    score: number;
    /** The one reason that matters most: "Smaller, same group", "Seeded 1,240". */
    reason: string;
    indexer?: string;
  }
</script>

<script lang="ts">
  import Button from '../../atoms/Button/Button.svelte';
  import Countdown from '../../atoms/Countdown/Countdown.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import { formatBytes, formatCount } from '../../../format/quantity';

  interface Props {
    variants: Variant[];
    /** Picked automatically when the countdown ends. */
    defaultId?: string;
    /** When the default is picked automatically; omit for no auto-pick. */
    autoAcceptAt?: Date | string | number;
    /** Resolution cap note: "Below 2160p". */
    cap?: string;
    title?: string;
    onpick?: (id: string, auto: boolean) => void;
    oncancel?: () => void;
    /** Offered when there is nothing to pick. */
    onsearch?: () => void;
    /** Script-free form target (Telegram widget). */
    formAction?: string;
    /** Fixed "now" (ms) for stories, tests and server rendering. */
    now?: number;
  }
  let {
    variants, defaultId = variants[0]?.id, autoAcceptAt, cap, title = 'Pick a replacement release', onpick, oncancel, onsearch,
    formAction, now,
  }: Props = $props();

  const uid = $props.id();
  // Seeded once from props: the owner's choice and the stopped timer must survive re-renders.
  // svelte-ignore state_referenced_locally
  let selected = $state(defaultId);
  // svelte-ignore state_referenced_locally
  let timer = $state(autoAcceptAt !== undefined);
  let done = false;

  const signed = (n: number) => (n > 0 ? `+${formatCount(n)}` : n < 0 ? `−${formatCount(-n)}` : '0');

  function pick(id: string | undefined, auto = false) {
    if (!id || done) return;
    done = true;
    timer = false;
    onpick?.(id, auto);
  }
  function onsubmit(e: SubmitEvent) {
    if (!onpick && !oncancel) return; // script-free post
    e.preventDefault();
    const which = (e.submitter as HTMLButtonElement | null)?.value;
    if (which === 'cancel') cancel();
    else pick(selected);
  }
  function cancel() {
    timer = false;
    oncancel?.();
  }
  function onchange() {
    timer = false;
  }
  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && oncancel) {
      e.preventDefault();
      cancel();
    }
  }
</script>

<!-- Escape from any control inside cancels (delegated); the form itself is not focusable. -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<form class="vp" method="post" action={formAction} {onsubmit} {onkeydown} aria-labelledby="{uid}-t">
  <div class="head">
    <h3 id="{uid}-t">{title}</h3>
    {#if cap}<span class="cap"><Icon name="lower" size="sm" />{cap}</span>{/if}
  </div>

  {#if !variants.length}
    <p class="none">No other releases fit. Search again to look for new ones.</p>
    {#if onsearch}<div class="actions"><Button icon="search" onclick={onsearch}>Search again</Button></div>{/if}
  {:else}
    <fieldset>
      <legend class="visually-hidden">Releases, best first</legend>
      {#each variants as v, i (v.id)}
        <label class={['opt', { chosen: selected === v.id }]}>
          <input type="radio" name="variant" value={v.id} bind:group={selected} {onchange} />
          <span class="rank num" aria-hidden="true">{i + 1}</span>
          <span class="text">
            <span class="top">
              <span class="quality">{v.quality}</span>
              {#if v.id === defaultId}<span class="default">Default</span>{/if}
            </span>
            <span class="name">{v.title}</span>
            <span class="facts">
              <span class="num">{formatBytes(v.size)}</span>
              <span class={['num', v.score > 0 ? 'pos' : v.score < 0 ? 'neg' : 'zero']}>Score {signed(v.score)}</span>
              {#if v.indexer}<span>{v.indexer}</span>{/if}
            </span>
            <span class="reason">{v.reason}</span>
          </span>
        </label>
      {/each}
    </fieldset>

    <div class="foot">
      <p class="auto">
        {#if timer && autoAcceptAt !== undefined}
          <Countdown to={autoAcceptAt} action="Picks the default" tone="info" {now} expiredLabel="picking now" onexpire={() => { if (timer) pick(defaultId, true); }} />
        {:else if autoAcceptAt !== undefined}
          <span class="stopped">Timer stopped. Pick when ready.</span>
        {/if}
      </p>
      <div class="actions">
        <Button type="submit" variant="ghost" name="action" value="cancel">Cancel</Button>
        <Button type="submit" variant="primary" name="action" value="pick">Use this release</Button>
      </div>
    </div>
  {/if}
</form>

<style>
  .vp {
    display: grid;
    gap: var(--space-3);
    width: 100%;
    max-width: 30rem;
    padding: var(--space-3);
    color: var(--ink);
    background: var(--surface-raised);
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-lg);
    font-size: var(--text-md);
  }
  .head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--space-2); }
  h3 { font-size: var(--text-base); }
  .cap { display: inline-flex; align-items: center; gap: var(--space-1); font-size: var(--text-sm); color: var(--ink-muted); }
  fieldset { display: grid; gap: var(--space-1-5); margin: 0; padding: 0; border: 0; min-width: 0; }
  .opt {
    display: grid;
    grid-template-columns: auto auto minmax(0, 1fr);
    gap: var(--space-2);
    align-items: start;
    padding: var(--space-2);
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-md);
    cursor: pointer;
  }
  .opt:hover { background: var(--surface-hover); }
  .opt.chosen { background: var(--surface-selected); border-color: var(--accent); }
  .opt:has(input:focus-visible) { outline: var(--focus-w) solid var(--focus-ring); outline-offset: var(--focus-offset); }
  input { margin: 0.1875rem 0 0; accent-color: var(--accent); }
  input:focus-visible { outline: none; }
  .rank { min-width: 1ch; font-size: var(--text-sm); color: var(--ink-subtle); line-height: 1.5; }
  .text { display: grid; gap: var(--space-0-5); min-width: 0; }
  .top { display: flex; align-items: center; gap: var(--space-2); }
  .quality { font-weight: var(--weight-semibold); }
  .default {
    padding: 0 var(--space-1-5);
    font-size: var(--text-xs);
    font-weight: var(--weight-medium);
    color: var(--accent-ink);
    background: var(--accent-tint);
    border-radius: var(--radius-sm);
  }
  .name { font-family: var(--font-mono); font-size: var(--text-xs); color: var(--ink-muted); overflow-wrap: anywhere; }
  .facts { display: flex; flex-wrap: wrap; gap: 0 var(--space-3); font-size: var(--text-sm); }
  .pos { color: var(--tone-ok-ink); }
  .neg { color: var(--tone-error-ink); }
  .zero { color: var(--ink-muted); }
  .reason { font-size: var(--text-sm); color: var(--ink-muted); }
  .foot { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--space-2); }
  .auto { min-height: 1lh; font-size: var(--text-sm); }
  .stopped { color: var(--ink-muted); }
  .actions { display: flex; gap: var(--space-2); margin-left: auto; }
  .none { color: var(--ink-muted); }

  @media (width < 48rem), (pointer: coarse) {
    .opt { min-height: var(--target-min); }
  }
</style>
