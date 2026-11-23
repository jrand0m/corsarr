<!--
  Textarea: multi-line text. `mono` for YAML, regex, paste-in titles and templates.
  Label and help come from the Field molecule.
-->
<script lang="ts">
  import type { HTMLTextareaAttributes } from 'svelte/elements';

  interface Props extends Omit<HTMLTextareaAttributes, 'value'> {
    value?: string;
    invalid?: boolean;
    mono?: boolean;
  }
  let { value = $bindable(''), invalid = false, mono = false, rows = 4, class: cls, ...rest }: Props = $props();
</script>

<textarea bind:value {rows} class={['ta', cls]} class:mono aria-invalid={invalid || undefined} {...rest}></textarea>

<style>
  .ta {
    display: block;
    width: 100%;
    padding: var(--space-2) var(--space-3);
    font-size: var(--text-md);
    line-height: var(--leading-normal);
    background: var(--surface-raised);
    border: var(--border-w) solid var(--line-strong);
    border-radius: var(--radius-md);
    resize: vertical;
  }
  .mono { font-family: var(--font-mono); font-size: var(--text-sm); tab-size: 2; }
  .ta:hover:not(:disabled, :read-only) { border-color: var(--ink-subtle); }
  .ta:focus-visible { outline-offset: 0; border-color: var(--focus-ring); }
  .ta[aria-invalid='true'] { border-color: var(--tone-error-solid); }
  .ta:read-only { background: var(--surface-sunken); border-style: dashed; }
  .ta:disabled { background: var(--surface-sunken); color: var(--ink-disabled); border-color: var(--line); }
  @media (width < 48rem), (pointer: coarse) { .ta { font-size: var(--text-base); } }
</style>
