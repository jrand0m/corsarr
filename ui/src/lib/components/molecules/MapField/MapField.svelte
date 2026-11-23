<!--
  MapField (schema field: map): key → value pairs (path mappings, extra headers, category map). Rows are
  edited in order; duplicate keys are flagged inline. The value is an ordered array of pairs
  so half-typed rows survive; the form serialises it to an object on save.
-->
<script lang="ts" module>
  export interface MapEntry { key: string; value: string }
  export function duplicateKeys(entries: MapEntry[]): Set<number> {
    const seen = new Map<string, number>();
    const dup = new Set<number>();
    entries.forEach((e, i) => {
      const k = e.key.trim();
      if (!k) return;
      if (seen.has(k)) { dup.add(i); dup.add(seen.get(k)!); } else seen.set(k, i);
    });
    return dup;
  }
</script>

<script lang="ts">
  import { tick } from 'svelte';
  import Field, { type FieldStateProps } from '../Field/Field.svelte';
  import TextInput from '../../atoms/TextInput/TextInput.svelte';
  import IconButton from '../../atoms/IconButton/IconButton.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';

  interface Props extends FieldStateProps {
    value?: MapEntry[];
    keyLabel?: string;
    valueLabel?: string;
    keyPlaceholder?: string;
    valuePlaceholder?: string;
    mono?: boolean;
    onchange?: (value: MapEntry[]) => void;
  }
  let {
    value = $bindable([]), keyLabel = 'Key', valueLabel = 'Value', keyPlaceholder, valuePlaceholder,
    mono = false, onchange, ...field
  }: Props = $props();

  let rows = $state<HTMLElement>();
  const dups = $derived(duplicateKeys(value));

  function set(i: number, part: 'key' | 'value', v: string) {
    value = value.map((e, j) => (j === i ? { ...e, [part]: v } : e));
    onchange?.(value);
  }
  async function add() {
    value = [...value, { key: '', value: '' }];
    onchange?.(value);
    await tick();
    rows?.querySelectorAll<HTMLElement>('.entry')[value.length - 1]?.querySelector('input')?.focus();
  }
  async function remove(i: number) {
    value = value.filter((_, j) => j !== i);
    onchange?.(value);
    await tick();
    rows?.querySelectorAll<HTMLElement>('.entry')[Math.min(i, value.length - 1)]?.querySelector('input')?.focus();
  }
</script>

<Field {...field} group>
  {#snippet children(c)}
    <div class="mf" bind:this={rows}>
      {#if value.length}
        <div class="cols" aria-hidden="true"><span>{keyLabel}</span><span>{valueLabel}</span></div>
        {#each value as e, i (i)}
          <div class="entry">
            <TextInput
              aria-label="{keyLabel} {i + 1}" value={e.key} {mono} placeholder={keyPlaceholder}
              invalid={dups.has(i)} readonly={c.readonly} disabled={c.disabled} spellcheck="false"
              aria-describedby={dups.has(i) ? `${c.id}-d${i}` : undefined}
              oninput={(ev) => set(i, 'key', (ev.currentTarget as HTMLInputElement).value)}
            />
            <span class="arrow" aria-hidden="true"><Icon name="arrow-right" size="sm" /></span>
            <TextInput
              aria-label="{valueLabel} {i + 1}" value={e.value} {mono} placeholder={valuePlaceholder}
              readonly={c.readonly} disabled={c.disabled} spellcheck="false"
              oninput={(ev) => set(i, 'value', (ev.currentTarget as HTMLInputElement).value)}
            />
            {#if !c.readonly}<IconButton icon="x" label="Remove row {i + 1}" disabled={c.disabled} onclick={() => remove(i)} />{/if}
            {#if dups.has(i)}<p class="err" id="{c.id}-d{i}">This {keyLabel.toLowerCase()} appears more than once</p>{/if}
          </div>
        {/each}
      {:else}
        <p class="none">No rows yet.</p>
      {/if}
      {#if !c.readonly}<span><Button size="sm" icon="plus" disabled={c.disabled} onclick={add}>Add row</Button></span>{/if}
    </div>
  {/snippet}
</Field>

<style>
  .mf { display: grid; gap: var(--space-1-5); }
  .cols, .entry { display: grid; grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr) var(--control-h-md); align-items: center; gap: var(--space-1) var(--space-2); }
  .cols { font-size: var(--text-xs); color: var(--ink-muted); }
  .cols span:last-child { grid-column: 3; }
  .arrow { color: var(--ink-subtle); }
  .err { grid-column: 1 / -1; font-size: var(--text-sm); color: var(--tone-error-ink); }
  .none { font-size: var(--text-sm); color: var(--ink-subtle); }
  @media (width < 48rem) {
    .cols { display: none; }
    .entry { grid-template-columns: minmax(0, 1fr) auto; padding-bottom: var(--space-2); border-bottom: var(--border-w) solid var(--line); }
    .arrow { display: none; }
    .entry > :global(:nth-child(3)) { grid-column: 1; }
    .entry > :global(button) { grid-row: 1 / span 2; grid-column: 2; }
  }
</style>
