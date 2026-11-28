<!--
  ManagedValue: a value the config sets, shown as "<value> <lock>". Hovering shows where it is set
  ("Managed by mediaKinds.tv.mode via environment variable CORSARR_MEDIAKINDS_TV_MODE") as a
  native tooltip; the same sentence is read out to assistive technology. Without a value only the
  lock shows (a form field whose control already shows the value).
-->
<script lang="ts" module>
  /** Where a managed value is set: the dotted key and the active config source. */
  export interface ManagedSource {
    key?: string;
    kind?: 'file' | 'args' | 'env' | string;
    /** The config file path (file source). */
    file?: string;
    /** The argument or environment variable name (args and env sources). */
    name?: string;
  }

  /** The source in words: config file with its path, command-line arguments, or the variable. */
  export function sourceText(m: ManagedSource): string {
    switch (m.kind) {
      case 'file': return m.file ? `config file ${m.file}` : 'the config file';
      case 'args': return m.name ? `command-line argument ${m.name}` : 'command-line arguments';
      case 'env': return m.name ? `environment variable ${m.name}` : 'environment variables';
      default: return 'the config';
    }
  }

  /** "Managed by <key> via <source>". */
  export function managedText(m: ManagedSource): string {
    return m.key ? `Managed by ${m.key} via ${sourceText(m)}` : `Managed via ${sourceText(m)}`;
  }

  /**
   * The managed marker of a key from the server's active config source, for screens that know
   * only the key. An environment source names the variable the key maps to.
   */
  export function managedFrom(key: string, source?: { kind?: string; path?: string }): ManagedSource {
    const kind = source?.kind;
    if (kind === 'file') return { key, kind, file: source?.path };
    if (kind === 'args') return { key, kind, name: `--${key}` };
    if (kind === 'env') return { key, kind, name: `CORSARR_${key.replace(/\./g, '_').toUpperCase()}` };
    return { key };
  }
</script>

<script lang="ts">
  import Icon from '../Icon/Icon.svelte';

  interface Props {
    /** The effective value in words ("Shadow"); omit to show the lock alone. */
    value?: string;
    managed: ManagedSource;
    size?: 'sm' | 'md';
  }
  let { value, managed, size = 'sm' }: Props = $props();
  const text = $derived(managedText(managed));
</script>

<span class={['mv', size]} title={text}>
  {#if value}<span class="value">{value}</span>{/if}
  <Icon name="lock" size="sm" />
  <span class="visually-hidden">({text})</span>
</span>

<style>
  .mv { display: inline-flex; align-items: center; gap: var(--space-1); color: var(--tone-managed-ink); white-space: nowrap; cursor: help; }
  .sm { font-size: var(--text-sm); }
  .md { font-size: var(--text-md); }
  .value { color: var(--ink); font-weight: var(--weight-medium); }
</style>
