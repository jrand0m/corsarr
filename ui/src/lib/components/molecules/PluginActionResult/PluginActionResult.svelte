<!--
  PluginActionResult: the answer to a plugin's settings button ("Test connection"): an outcome
  badge (running, OK, warning, failed, or not shown when the plugin's text broke the plain-text
  rules) and the plugin's text as a plain paragraph, line breaks kept. A polite live region, so
  the result is read out when it arrives.
-->
<script lang="ts" module>
  import type { ActionOutcome } from '../../../forms/plugin-types';

  export type ActionState = 'running' | ActionOutcome | 'refused';
  /** The fixed text shown when the server refused the plugin's answer. */
  export const REFUSED_TEXT = 'The plugin answered with text corsarr cannot show.';
</script>

<script lang="ts">
  import StatusBadge from '../../atoms/StatusBadge/StatusBadge.svelte';

  interface Props {
    state: ActionState;
    /** The plugin's text (plain); ignored while running and when refused. */
    text?: string;
    /** The button that ran: "Test connection". */
    action?: string;
  }
  let { state, text, action }: Props = $props();

  const shown = $derived(state === 'refused' ? REFUSED_TEXT : state === 'running' ? (action ? `${action}…` : 'Waiting for the plugin…') : text);
</script>

<div class={['result', state]} role="status" aria-live="polite">
  <StatusBadge family="actionOutcome" value={state} size="sm" />
  {#if action && state !== 'running'}<span class="action">{action}</span>{/if}
  {#if shown}<p class="says">{shown}</p>{/if}
</div>

<style>
  .result {
    display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--space-1-5) var(--space-2);
    padding: var(--space-2) var(--space-3); border: var(--border-w) solid var(--line); border-radius: var(--radius-md);
    background: var(--surface-sunken); min-width: 0;
  }
  .action { font-size: var(--text-sm); color: var(--ink-muted); font-weight: var(--weight-medium); }
  .says { flex-basis: 100%; font-size: var(--text-md); color: var(--ink); white-space: pre-line; overflow-wrap: anywhere; max-width: var(--measure); }
  .refused .says { color: var(--ink-muted); }
</style>
