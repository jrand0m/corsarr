<!--
  ConfirmDialog: the stop before a destructive action. Names what will happen as a
  list of consequences, offers a choice when the action has variants (kill: "blocklist and
  search again" or "just remove"), and says plainly whether it can be undone. Focus
  starts on Cancel, so Enter never destroys by accident. The confirmed action ends in a
  ToastRegion toast with Undo, which the caller raises from `onconfirm`.
-->
<script lang="ts" module>
  export interface ConfirmChoice {
    value: string;
    label: string;
    description?: string;
    /** Consequences added to the list while this choice is selected. */
    consequences?: string[];
  }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import Dialog from '../Dialog/Dialog.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import Radio from '../../atoms/Radio/Radio.svelte';

  interface Props {
    open?: boolean;
    /** The question, naming the object: "Kill The Long Haul S02E03?". */
    title: string;
    /** What happens, in order, in plain words. */
    consequences?: string[];
    choices?: ConfirmChoice[];
    /** Legend of the choice group. */
    choiceLabel?: string;
    choice?: string;
    /** true: undoable (default wording); false: permanent; a string: the exact undo statement. */
    undoable?: boolean | string;
    confirmLabel: string;
    cancelLabel?: string;
    /** `danger` for destructive actions (default); `primary` for weighty but safe ones. */
    variant?: 'danger' | 'primary';
    loading?: boolean;
    /** Extra options under the choices ("Also delete the files"). */
    children?: Snippet;
    onconfirm?: (choice: string | undefined) => void;
    oncancel?: () => void;
    modal?: boolean;
  }
  let {
    open = $bindable(false), title, consequences = [], choices, choiceLabel = 'Then', choice = $bindable(choices?.[0]?.value),
    undoable = true, confirmLabel, cancelLabel = 'Cancel', variant = 'danger', loading = false, children, onconfirm,
    oncancel, modal = true,
  }: Props = $props();

  const selected = $derived(choices?.find((c) => c.value === choice));
  const list = $derived([...consequences, ...(selected?.consequences ?? [])]);
  const undoText = $derived(
    typeof undoable === 'string' ? undoable
      : undoable ? 'You can undo this from the notification, or from History for 30 days.'
        : 'This is permanent and cannot be undone.',
  );
  const uid = $props.id();
  let confirmed = false;

  function confirm() {
    confirmed = true;
    onconfirm?.(choice);
    open = false;
  }
  function closed() {
    if (!confirmed) oncancel?.();
    confirmed = false;
  }
</script>

<Dialog bind:open {title} role="alertdialog" size="sm" {modal} onclose={closed} dismissible={!loading}>
  <div class="content">
    {#if list.length}
      <ul class="consequences" aria-label="What happens">
        {#each list as line (line)}
          <li><Icon name="arrow-right" size="sm" /><span>{line}</span></li>
        {/each}
      </ul>
    {/if}

    {#if choices?.length}
      <fieldset>
        <legend>{choiceLabel}</legend>
        {#each choices as c (c.value)}
          <Radio bind:group={choice} value={c.value} label={c.label} description={c.description} name="{uid}-choice" />
        {/each}
      </fieldset>
    {/if}

    {#if children}<div class="extra">{@render children()}</div>{/if}

    <p class={['undo', undoable === false ? 'permanent' : 'undoable']}>
      <Icon name={undoable === false ? 'alert-octagon' : 'undo'} size="sm" />
      <span>{undoText}</span>
    </p>
  </div>

  {#snippet actions()}
    <Button variant="ghost" data-autofocus disabled={loading} onclick={() => (open = false)}>{cancelLabel}</Button>
    <Button {variant} {loading} onclick={confirm}>{confirmLabel}</Button>
  {/snippet}
</Dialog>

<style>
  .content { display: grid; gap: var(--space-4); }
  .consequences { display: grid; gap: var(--space-1-5); padding: 0; list-style: none; }
  .consequences li { display: flex; gap: var(--space-2); align-items: flex-start; line-height: var(--leading-snug); }
  .consequences li :global(svg) { margin-top: 0.1875rem; color: var(--ink-muted); }
  fieldset { display: grid; gap: var(--space-2); padding: 0; border: 0; }
  legend { padding: 0; margin-bottom: var(--space-2); font-size: var(--text-md); font-weight: var(--weight-semibold); }
  .extra { display: grid; gap: var(--space-2); }
  .undo {
    display: flex;
    gap: var(--space-2);
    align-items: flex-start;
    padding: var(--space-2) var(--space-3);
    font-size: var(--text-sm);
    line-height: var(--leading-snug);
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-md);
  }
  .undo :global(svg) { margin-top: 0.125rem; }
  .undoable { color: var(--ink-muted); background: var(--surface-sunken); }
  .permanent { color: var(--tone-error-ink); background: var(--tone-error-tint); border-color: var(--tone-error-line); }
</style>
