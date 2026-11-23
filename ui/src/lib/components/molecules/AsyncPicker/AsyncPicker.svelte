<!--
  AsyncPicker: find and pick items, quality profiles, tags or plugins from a list that
  lives on the server. An ARIA 1.2 combobox: type to search (debounced, earlier requests
  aborted), arrows move, Enter picks, Escape closes, Backspace on an empty query removes the
  last pick in `multiple` mode. Loading, empty and error states are spoken. The popup is in
  the top layer (popover), so drawers and tables never clip it.

  `value` is always an array of picked options (one at most unless `multiple`), so the caller
  keeps labels for values that are not in the current results.
-->
<script lang="ts" module>
  import type { IconName } from '../../atoms/Icon/icons';
  import type { MediaKind } from '../../../tokens';
  export interface PickerOption {
    value: string;
    label: string;
    description?: string;
    icon?: IconName;
    kind?: MediaKind;
    disabled?: boolean;
  }
  export type PickerSearch = (query: string, signal: AbortSignal) => Promise<PickerOption[]>;
</script>

<script lang="ts">
  import Icon from '../../atoms/Icon/Icon.svelte';
  import Spinner from '../../atoms/Spinner/Spinner.svelte';
  import Chip from '../../atoms/Chip/Chip.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import MediaKindIcon from '../../atoms/MediaKindIcon/MediaKindIcon.svelte';

  interface Props {
    search: PickerSearch;
    value?: PickerOption[];
    multiple?: boolean;
    /** Accessible name when no Field supplies a label. */
    label?: string;
    /** Plural noun for messages: "profiles", "series". */
    noun?: string;
    placeholder?: string;
    /** Characters before searching; 0 lists options on focus. */
    minChars?: number;
    debounce?: number;
    id?: string;
    describedby?: string;
    invalid?: boolean;
    readonly?: boolean;
    disabled?: boolean;
    required?: boolean;
    onchange?: (value: PickerOption[]) => void;
  }
  let {
    search, value = $bindable([]), multiple = false, label, noun = 'results', placeholder, minChars = 0, debounce = 200,
    id, describedby, invalid = false, readonly = false, disabled = false, required = false, onchange,
  }: Props = $props();

  const uid = $props.id();
  const listId = `${uid}-list`;
  // svelte-ignore state_referenced_locally
  let query = $state(!multiple && value[0] ? value[0].label : '');
  let open = $state(false);
  let phase = $state<'idle' | 'loading' | 'done' | 'error'>('idle');
  let results = $state<PickerOption[]>([]);
  let errorText = $state('');
  let active = $state(-1);
  let input = $state<HTMLInputElement>();
  let box = $state<HTMLElement>();
  let pos = $state({ top: 0, left: 0, width: 0 });
  let timer: ReturnType<typeof setTimeout> | undefined;
  let ctrl: AbortController | undefined;

  const picked = $derived(new Set(value.map((v) => v.value)));
  const tooShort = $derived(query.trim().length < minChars);

  function run(q: string) {
    clearTimeout(timer);
    ctrl?.abort();
    if (q.trim().length < minChars) { phase = 'idle'; results = []; return; }
    phase = 'loading';
    timer = setTimeout(async () => {
      const c = (ctrl = new AbortController());
      try {
        const r = await search(q.trim(), c.signal);
        if (c.signal.aborted) return;
        results = r;
        phase = 'done';
        active = r.findIndex((o) => !o.disabled);
      } catch (e) {
        if (c.signal.aborted) return;
        errorText = (e as Error)?.message || 'the server did not answer';
        phase = 'error';
      }
    }, debounce);
  }
  $effect(() => () => { clearTimeout(timer); ctrl?.abort(); });

  function show() {
    if (readonly || disabled) return;
    if (!open) { open = true; run(multiple || !value[0] || query !== value[0].label ? query : ''); }
  }
  function close() {
    open = false;
    active = -1;
    ctrl?.abort();
    clearTimeout(timer);
    if (!multiple) query = value[0]?.label ?? '';
  }
  function pick(o: PickerOption) {
    if (o.disabled) return;
    if (multiple) {
      value = picked.has(o.value) ? value.filter((v) => v.value !== o.value) : [...value, o];
      query = '';
      run('');
    } else {
      value = [o];
      query = o.label;
      open = false;
    }
    onchange?.(value);
  }
  function remove(o: PickerOption) {
    value = value.filter((v) => v.value !== o.value);
    onchange?.(value);
    input?.focus();
  }
  function move(step: number) {
    if (!results.length) return;
    let i = active;
    for (let n = 0; n < results.length; n++) {
      i = (i + step + results.length) % results.length;
      if (!results[i].disabled) break;
    }
    active = i;
    box?.querySelector(`#${CSS.escape(`${uid}-o${i}`)}`)?.scrollIntoView?.({ block: 'nearest' });
  }
  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') { e.preventDefault(); if (!open) show(); else move(1); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); if (open) move(-1); }
    else if (e.key === 'Enter' && open && active >= 0 && results[active]) { e.preventDefault(); pick(results[active]); }
    else if (e.key === 'Escape' && open) { e.preventDefault(); close(); }
    else if (e.key === 'Backspace' && multiple && query === '' && value.length) remove(value[value.length - 1]);
  }
  function oninput() {
    if (!open) open = true;
    run(query);
  }
  function place(el: HTMLElement) {
    const anchor = input?.closest('.ap') as HTMLElement | null;
    if (!anchor) return;
    const r = anchor.getBoundingClientRect();
    const below = r.bottom + 4 + el.offsetHeight <= innerHeight || r.top < el.offsetHeight;
    pos = { top: below ? r.bottom + 4 : r.top - 4 - el.offsetHeight, left: r.left, width: r.width };
  }
  function popup(el: HTMLElement) {
    el.showPopover?.();
    place(el);
    const update = () => place(el);
    addEventListener('scroll', update, true);
    addEventListener('resize', update);
    return () => { removeEventListener('scroll', update, true); removeEventListener('resize', update); };
  }
  const message = $derived(
    phase === 'loading' ? `Searching ${noun}…`
      : phase === 'error' ? `Couldn't search ${noun}: ${errorText}`
      : phase === 'done' && results.length === 0 ? (query.trim() ? `No ${noun} match “${query.trim()}”` : `No ${noun} yet`)
      : tooShort && minChars > 0 ? `Type ${minChars} or more characters to search ${noun}`
      : '',
  );
</script>

<div class="ap" class:invalid class:off={disabled} class:locked={readonly}>
  {#if multiple}
    {#each value as v (v.value)}
      <Chip label={v.label} onremove={readonly || disabled ? undefined : () => remove(v)} />
    {/each}
  {/if}
  <span class="in">
    <Icon name="search" size="sm" />
    <input
      bind:this={input} bind:value={query} {id} type="text" role="combobox" autocomplete="off" spellcheck="false"
      aria-label={id ? undefined : label} aria-describedby={describedby} aria-invalid={invalid || undefined}
      aria-required={required || undefined} aria-expanded={open} aria-controls={listId} aria-autocomplete="list"
      aria-activedescendant={open && active >= 0 ? `${uid}-o${active}` : undefined}
      placeholder={multiple && value.length ? '' : placeholder} {readonly} {disabled}
      onfocus={show} onclick={show} onblur={(e) => { if (!box?.contains(e.relatedTarget as Node)) close(); }}
      {onkeydown} {oninput}
    />
    {#if phase === 'loading' && open}<Spinner size="sm" />{/if}
  </span>
</div>
<span class="visually-hidden" role="status">{open ? message || (phase === 'done' ? `${results.length} ${noun}` : '') : ''}</span>

{#if open}
  <div
    class="pop" popover="manual" bind:this={box} {@attach popup}
    style:top="{pos.top}px" style:left="{pos.left}px" style:width="{pos.width}px"
  >
    <ul id={listId} role="listbox" aria-label={label ?? noun} aria-multiselectable={multiple || undefined}>
      {#each results as o, i (o.value)}
        <!-- Keyboard reaches options through the combobox (aria-activedescendant); clicks are for pointers. -->
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <li
          id="{uid}-o{i}" role="option" aria-selected={picked.has(o.value)} aria-disabled={o.disabled || undefined}
          class:active={i === active} onpointerdown={(e) => e.preventDefault()} onclick={() => pick(o)}
          onpointermove={() => { if (!o.disabled) active = i; }}
        >
          {#if o.kind}<MediaKindIcon kind={o.kind} size="sm" />{:else if o.icon}<Icon name={o.icon} size="sm" />{/if}
          <span class="txt"><span class="lab">{o.label}</span>{#if o.description}<span class="desc">{o.description}</span>{/if}</span>
          {#if picked.has(o.value)}<Icon name="check" size="sm" />{/if}
        </li>
      {/each}
    </ul>
    {#if message}
      <div class={['msg', phase]}>
        {#if phase === 'loading'}<Spinner size="sm" />{:else if phase === 'error'}<Icon name="alert-octagon" size="sm" />{/if}
        <span>{message}</span>
        {#if phase === 'error'}<Button size="sm" variant="ghost" onclick={() => run(query)} onpointerdown={(e: PointerEvent) => e.preventDefault()}>Retry</Button>{/if}
      </div>
    {/if}
  </div>
{/if}

<style>
  .ap {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1);
    min-height: var(--control-h-md);
    padding: var(--space-0-5) var(--space-1);
    background: var(--surface-raised);
    border: var(--border-w) solid var(--line-strong);
    border-radius: var(--radius-md);
    transition: border-color var(--dur-fast) var(--ease-out);
  }
  .ap:hover:not(.off, .locked) { border-color: var(--ink-subtle); }
  .ap:focus-within { border-color: var(--focus-ring); outline: var(--focus-w) solid var(--focus-ring); outline-offset: 0; }
  .invalid { border-color: var(--tone-error-solid); }
  .locked { background: var(--surface-sunken); border-style: dashed; }
  .off { background: var(--surface-sunken); border-color: var(--line); color: var(--ink-disabled); }
  .in { position: relative; display: flex; align-items: center; gap: var(--space-1-5); flex: 1 1 8rem; min-width: 6rem; padding-left: var(--space-1-5); color: var(--ink-subtle); }
  input { flex: 1; min-width: 0; min-height: calc(var(--control-h-md) - var(--space-1) - 2 * var(--border-w)); padding: 0; font-size: var(--text-md); color: var(--ink); background: none; border: 0; }
  input:focus-visible { outline: none; }
  .pop {
    position: fixed;
    inset: auto;
    margin: 0;
    max-height: min(20rem, 50vh);
    overflow: auto;
    padding: var(--space-1);
    color: var(--ink);
    background: var(--surface-overlay);
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-sm);
  }
  ul { display: grid; margin: 0; padding: 0; list-style: none; }
  li { display: flex; align-items: center; gap: var(--space-2); min-height: var(--control-h-md); padding: var(--space-1) var(--space-2); border-radius: var(--radius-sm); cursor: pointer; }
  li.active { background: var(--surface-hover); outline: var(--border-w) solid var(--line-strong); }
  li[aria-selected='true'] { color: var(--accent-ink); }
  li[aria-disabled='true'] { color: var(--ink-disabled); cursor: not-allowed; }
  li :global(svg) { flex: none; }
  .txt { display: grid; flex: 1; min-width: 0; }
  .lab { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--text-md); }
  .desc { font-size: var(--text-xs); color: var(--ink-muted); }
  .msg { display: flex; align-items: center; gap: var(--space-2); padding: var(--space-2); font-size: var(--text-sm); color: var(--ink-muted); }
  .msg.error { color: var(--tone-error-ink); }
  @media (width < 48rem), (pointer: coarse) {
    .ap { min-height: var(--target-min); }
    input { font-size: var(--text-base); }
    li { min-height: var(--target-min); }
  }
</style>
