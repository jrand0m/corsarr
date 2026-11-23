<!--
  ListField (schema field: list): an unordered list of strings (tags, extra trackers, ignored words).
  Each entry is its own input with a remove button; Enter in the last input adds a new one.
  Order is kept but not edited here; ordered lists use OrderedListEditor.
-->
<script lang="ts">
  import { tick } from 'svelte';
  import Field, { type FieldStateProps } from '../Field/Field.svelte';
  import TextInput from '../../atoms/TextInput/TextInput.svelte';
  import IconButton from '../../atoms/IconButton/IconButton.svelte';
  import Button from '../../atoms/Button/Button.svelte';

  interface Props extends FieldStateProps {
    value?: string[];
    /** Name of one entry, used in button labels: "tracker", "word". */
    itemLabel?: string;
    placeholder?: string;
    mono?: boolean;
    max?: number;
    /** Per-entry errors from the validator, by index. */
    itemErrors?: Record<number, string>;
    onchange?: (value: string[]) => void;
  }
  let {
    value = $bindable([]), itemLabel = 'entry', placeholder, mono = false, max, itemErrors = {}, onchange, ...field
  }: Props = $props();

  let list = $state<HTMLElement>();
  const full = $derived(max !== undefined && value.length >= max);

  async function add() {
    if (full) return;
    value = [...value, ''];
    onchange?.(value);
    await tick();
    list?.querySelectorAll<HTMLInputElement>('input')[value.length - 1]?.focus();
  }
  async function remove(i: number) {
    value = value.filter((_, j) => j !== i);
    onchange?.(value);
    await tick();
    const inputs = list?.querySelectorAll<HTMLInputElement>('input');
    (inputs?.[Math.min(i, value.length - 1)] ?? list?.parentElement?.querySelector<HTMLButtonElement>('.add button, button.add'))?.focus();
  }
  function set(i: number, v: string) {
    value = value.map((x, j) => (j === i ? v : x));
    onchange?.(value);
  }
</script>

<Field {...field} group>
  {#snippet children(c)}
    <div class="lf">
      {#if value.length}
        <ul bind:this={list}>
          {#each value as item, i (i)}
            <li>
              <div class="row">
                <TextInput
                  aria-label="{field.label}, {itemLabel} {i + 1} of {value.length}" value={item} {mono} {placeholder}
                  invalid={!!itemErrors[i]} readonly={c.readonly} disabled={c.disabled} spellcheck="false"
                  aria-describedby={itemErrors[i] ? `${c.id}-e${i}` : undefined}
                  oninput={(e) => set(i, (e.currentTarget as HTMLInputElement).value)}
                  onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); if (i === value.length - 1) add(); } }}
                />
                {#if !c.readonly}
                  <IconButton icon="x" label="Remove {itemLabel} {i + 1}" disabled={c.disabled} onclick={() => remove(i)} />
                {/if}
              </div>
              {#if itemErrors[i]}<p class="err" id="{c.id}-e{i}">{itemErrors[i]}</p>{/if}
            </li>
          {/each}
        </ul>
      {:else}
        <p class="none">No {itemLabel}s yet.</p>
      {/if}
      {#if !c.readonly}
        <span><Button size="sm" icon="plus" class="add" disabled={c.disabled || full} onclick={add}>Add {itemLabel}</Button></span>
      {/if}
    </div>
  {/snippet}
</Field>

<style>
  .lf { display: grid; gap: var(--space-2); justify-items: start; }
  ul { display: grid; gap: var(--space-1-5); width: 100%; padding: 0; list-style: none; }
  .row { display: flex; align-items: center; gap: var(--space-1); }
  .none { font-size: var(--text-sm); color: var(--ink-subtle); }
  .err { margin-top: var(--space-1); font-size: var(--text-sm); color: var(--tone-error-ink); }
</style>
