<!--
  CopyableValue: a value the owner copies elsewhere (API key, iCal or Torznab URL,
  diagnostics). `secret` masks it until revealed. Copy confirms in place for 2 s and announces
  "Copied" politely. If the clipboard API is unavailable, the value is selected instead.
-->
<script lang="ts">
  import IconButton from '../IconButton/IconButton.svelte';

  interface Props {
    value: string;
    /** Accessible name of the value: "API key", "iCal URL". */
    label: string;
    secret?: boolean;
    mono?: boolean;
    oncopy?: (value: string) => void;
  }
  let { value, label, secret = false, mono = true, oncopy }: Props = $props();

  let revealed = $state(false);
  let copied = $state(false);
  let el = $state<HTMLElement>();
  let timer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => () => clearTimeout(timer));

  const shown = $derived(secret && !revealed ? '•'.repeat(Math.min(24, Math.max(8, value.length))) : value);

  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
    } catch {
      revealed = true;
      queueMicrotask(() => {
        if (!el) return;
        const range = document.createRange();
        range.selectNodeContents(el);
        getSelection()?.removeAllRanges();
        getSelection()?.addRange(range);
      });
      return;
    }
    copied = true;
    oncopy?.(value);
    clearTimeout(timer);
    timer = setTimeout(() => (copied = false), 2000);
  }
</script>

<span class="cv">
  <span class="value" class:mono class:masked={secret && !revealed} bind:this={el} aria-label={secret && !revealed ? `${label}, hidden` : undefined}>{shown}</span>
  {#if secret}
    <IconButton size="sm" icon={revealed ? 'eye-off' : 'eye'} label={revealed ? `Hide ${label}` : `Show ${label}`} pressed={revealed} onclick={() => (revealed = !revealed)} />
  {/if}
  <IconButton size="sm" icon={copied ? 'check' : 'copy'} label={copied ? 'Copied' : `Copy ${label}`} onclick={copy} />
  <span class="visually-hidden" role="status">{copied ? 'Copied' : ''}</span>
</span>

<style>
  .cv {
    display: inline-flex;
    align-items: center;
    gap: var(--space-0-5);
    max-width: 100%;
    min-height: var(--control-h-md);
    padding: 0 var(--space-0-5) 0 var(--space-2);
    background: var(--surface-sunken);
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-md);
  }
  .value { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--text-md); padding-right: var(--space-1); }
  .mono { font-family: var(--font-mono); font-size: var(--text-sm); }
  .masked { letter-spacing: 0.1em; color: var(--ink-muted); }
</style>
