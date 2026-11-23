<!--
  PathPicker: choose a folder corsarr can write to. Known locations (root folders,
  download folders) show their free space and whether corsarr can reach and write them; the
  owner picks one or types another path. Native radios, so it works by keyboard and on phones.
  With no known locations it is a plain path input.
-->
<script lang="ts" module>
  export interface PathLocation {
    path: string;
    /** Friendly name: "TV library", "Downloads (qBittorrent)". */
    label?: string;
    free?: number;
    total?: number;
    access?: 'writable' | 'read-only' | 'unreachable';
    /** Why it is not writable: "permission denied for uid 1000". */
    problem?: string;
  }
</script>

<script lang="ts">
  import Meter from '../../atoms/Meter/Meter.svelte';
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';
  import TextInput from '../../atoms/TextInput/TextInput.svelte';
  import { formatBytes } from '../../../format/quantity';

  interface Props {
    value?: string;
    locations?: PathLocation[];
    /** Accessible name when no Field supplies a label. */
    label?: string;
    allowOther?: boolean;
    placeholder?: string;
    id?: string;
    describedby?: string;
    invalid?: boolean;
    readonly?: boolean;
    disabled?: boolean;
    required?: boolean;
    onchange?: (value: string) => void;
  }
  let {
    value = $bindable(''), locations = [], label, allowOther = true, placeholder = '/data/media/tv',
    id, describedby, invalid = false, readonly = false, disabled = false, required = false, onchange,
  }: Props = $props();

  const name = $props.id();
  const known = $derived(locations.some((l) => l.path === value));
  // svelte-ignore state_referenced_locally
  let other = $state(value && !locations.some((l) => l.path === value) ? value : '');
  let otherMode = $state(false);
  const otherChecked = $derived(!known && (otherMode || value !== ''));

  function pick(path: string) {
    otherMode = false;
    value = path;
    onchange?.(value);
  }
  function useOther() {
    otherMode = true;
    value = other;
    onchange?.(value);
  }
  const used = (l: PathLocation) => (l.free !== undefined && l.total ? 1 - l.free / l.total : undefined);
</script>

{#if locations.length === 0}
  <TextInput
    {id} aria-describedby={describedby} aria-label={label} mono {invalid} {readonly} {disabled} {required}
    bind:value {placeholder} spellcheck="false" autocomplete="off" onchange={() => onchange?.(value)}
  />
{:else}
  <div class="pp" role="radiogroup" aria-label={label} aria-describedby={describedby} aria-invalid={invalid || undefined} aria-required={required || undefined} {id}>
    {#each locations as l (l.path)}
      {@const u = used(l)}
      <label class="loc" class:on={value === l.path} class:bad={l.access && l.access !== 'writable'} class:off={disabled || (readonly && value !== l.path)}>
        <input
          type="radio" {name} value={l.path} checked={value === l.path}
          disabled={disabled || (readonly && value !== l.path)} onchange={() => pick(l.path)}
        />
        <span class="body">
          <span class="top">
            <code class="path">{l.path}</code>
            {#if l.access && l.access !== 'writable'}<StatusBadge family="pathAccess" value={l.access} size="sm" detail={l.problem} />{/if}
          </span>
          {#if l.label}<span class="name">{l.label}</span>{/if}
          {#if u !== undefined}
            <Meter
              label="Space used on {l.path}" value={u} better="low" warnAt={0.85} errorAt={0.95} showValue
              valueText="{formatBytes(l.free!)} free of {formatBytes(l.total!)}"
            />
          {/if}
        </span>
      </label>
    {/each}
    {#if allowOther}
      <label class="loc other" class:on={otherChecked} class:off={disabled || readonly}>
        <input type="radio" {name} value={other} checked={otherChecked} disabled={disabled || readonly} onchange={(e) => { useOther(); e.currentTarget.closest('label')?.querySelector<HTMLInputElement>('input:not([type=radio])')?.focus(); }} />
        <span class="body">
          <span class="name">Another folder</span>
          <TextInput
            aria-label="Another folder path" mono {invalid} disabled={disabled || readonly} bind:value={other}
            {placeholder} spellcheck="false" autocomplete="off"
            oninput={useOther}
          />
        </span>
      </label>
    {/if}
  </div>
{/if}

<style>
  .pp { display: grid; gap: var(--space-2); }
  .loc {
    display: flex;
    align-items: flex-start;
    gap: var(--space-3);
    padding: var(--space-3);
    background: var(--surface-raised);
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-md);
    cursor: pointer;
    transition: border-color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out);
  }
  .loc:hover:not(.off) { border-color: var(--line-strong); }
  .loc.on { border-color: var(--accent); background: var(--surface-selected); }
  .loc.off { cursor: not-allowed; color: var(--ink-disabled); }
  input[type='radio'] { margin: 0.1875rem 0 0; flex: none; width: 1rem; height: 1rem; accent-color: var(--accent); }
  .body { display: grid; gap: var(--space-1-5); flex: 1; min-width: 0; }
  .top { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-1) var(--space-2); }
  .path { font-size: var(--text-sm); overflow-wrap: anywhere; }
  .name { font-size: var(--text-sm); color: var(--ink-muted); }
  .bad .path { color: var(--ink-muted); }
  @media (width < 48rem), (pointer: coarse) {
    .loc { min-height: var(--target-min); }
  }
</style>
