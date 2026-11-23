<!--
  OrderedListEditor: an ordered list the user ranks, by drag or by keyboard, with optional
  groups of equal rank (a quality ladder: "Bluray-1080p and WEB-1080p count the same").

  Keyboard on a row's handle: Space lifts the row, arrows move it, Space drops, Escape puts it
  back; Alt+arrow moves at once; plain arrows move between handles. Every row also has a menu
  (move to top/bottom, group with the next row, ungroup). Moves are announced.
-->
<script lang="ts" module>
  export interface OrderedItem {
    id: string;
    label: string;
    detail?: string;
    enabled?: boolean;
    /** Why this row cannot be switched (a floor, a lock); keeps its place in the order. */
    locked?: string;
    /** A group: these items share one rank. */
    children?: OrderedItem[];
  }

  const clone = (items: OrderedItem[]): OrderedItem[] => items.map((i) => ({ ...i, children: i.children?.map((c) => ({ ...c })) }));

  /** Move within the top level (parent undefined) or within one group. */
  export function moveItem(items: OrderedItem[], parent: string | undefined, from: number, to: number): OrderedItem[] {
    const next = clone(items);
    const list = parent ? next.find((i) => i.id === parent)?.children : next;
    if (!list || to < 0 || to >= list.length || from === to) return next;
    const [it] = list.splice(from, 1);
    list.splice(to, 0, it);
    return next;
  }

  /** Merge the row at `index` with the one below into one group (or into the group below). */
  export function groupWithNext(items: OrderedItem[], index: number, groupId = `group-${items[index]?.id}`): OrderedItem[] {
    const next = clone(items);
    const a = next[index], b = next[index + 1];
    if (!a || !b) return next;
    const members = [...(a.children ?? [a]), ...(b.children ?? [b])];
    const label = a.children ? a.label : `${a.label} + ${b.children ? b.label : b.label}`;
    next.splice(index, 2, { id: a.children ? a.id : groupId, label, children: members, enabled: members.some((m) => m.enabled !== false) });
    return next;
  }

  /** Dissolve the group at `index` into rows in its place. */
  export function ungroup(items: OrderedItem[], index: number): OrderedItem[] {
    const next = clone(items);
    const g = next[index];
    if (!g?.children) return next;
    next.splice(index, 1, ...g.children);
    return next;
  }
</script>

<script lang="ts">
  import { tick } from 'svelte';
  import Checkbox from '../../atoms/Checkbox/Checkbox.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import ActionMenu, { type ActionItem } from '../../molecules/ActionMenu/ActionMenu.svelte';
  import type { Density } from '../../../tokens';

  interface Props {
    items?: OrderedItem[];
    /** Accessible name: "Quality order for HD". */
    label: string;
    /** What the order means, shown above the list: "Top is most preferred". */
    hint?: string;
    toggleable?: boolean;
    groupable?: boolean;
    readonly?: boolean;
    density?: Density;
    onchange?: (items: OrderedItem[]) => void;
  }
  let {
    items = $bindable([]), label, hint, toggleable = false, groupable = false, readonly = false,
    density = 'comfortable', onchange,
  }: Props = $props();

  const id = $props.id();
  let root = $state<HTMLElement>();
  let grabbed = $state<{ id: string; parent?: string; snapshot: OrderedItem[] }>();
  let announcement = $state('');
  let drag = $state<{ id: string; parent?: string; from: number }>();
  let over = $state<string>();

  function commit(next: OrderedItem[]) {
    items = next;
    onchange?.(next);
  }
  const listOf = (parent?: string) => (parent ? items.find((i) => i.id === parent)?.children ?? [] : items);
  const nameOf = (it: OrderedItem) => it.label;

  async function focusHandle(itemId: string) {
    await tick();
    root?.querySelector<HTMLElement>(`[data-handle="${CSS.escape(itemId)}"]`)?.focus();
  }
  function announcePos(it: OrderedItem, parent: string | undefined) {
    const list = listOf(parent);
    const i = list.findIndex((x) => x.id === it.id);
    announcement = `${nameOf(it)}, position ${i + 1} of ${list.length}${parent ? ` in ${listOf().find((g) => g.id === parent)?.label}` : ''}`;
  }
  function move(it: OrderedItem, parent: string | undefined, to: number) {
    const list = listOf(parent);
    const from = list.findIndex((x) => x.id === it.id);
    if (to < 0 || to >= list.length || to === from) return;
    commit(moveItem(items, parent, from, to));
    announcePos(it, parent);
    focusHandle(it.id);
  }

  function onhandlekey(e: KeyboardEvent, it: OrderedItem, parent: string | undefined, index: number) {
    if (readonly) return;
    const list = listOf(parent);
    if (e.key === ' ' || e.key === 'Enter') {
      e.preventDefault();
      if (grabbed?.id === it.id) {
        grabbed = undefined;
        announcement = `${nameOf(it)} dropped at position ${index + 1} of ${list.length}`;
      } else {
        grabbed = { id: it.id, parent, snapshot: clone(items) };
        announcement = `${nameOf(it)} lifted, position ${index + 1} of ${list.length}. Use the arrow keys to move, Space to drop, Escape to cancel.`;
      }
      return;
    }
    if (e.key === 'Escape' && grabbed?.id === it.id) {
      e.preventDefault();
      commit(grabbed.snapshot);
      grabbed = undefined;
      announcement = `Move cancelled. ${nameOf(it)} is back at its place.`;
      focusHandle(it.id);
      return;
    }
    const step = e.key === 'ArrowUp' ? -1 : e.key === 'ArrowDown' ? 1 : e.key === 'Home' ? -Infinity : e.key === 'End' ? Infinity : 0;
    if (!step) return;
    e.preventDefault();
    const target = Math.max(0, Math.min(list.length - 1, index + step));
    if (grabbed?.id === it.id || e.altKey) move(it, parent, target);
    else focusHandle(list[target].id);
  }

  // A move re-inserts the row, which blurs the handle for a moment; drop only when focus really left.
  function dropOnLeave(itemId: string) {
    setTimeout(() => {
      if (grabbed?.id === itemId && (document.activeElement as HTMLElement | null)?.dataset.handle !== itemId) grabbed = undefined;
    });
  }

  function menu(it: OrderedItem, parent: string | undefined, index: number): ActionItem[] {
    const n = listOf(parent).length;
    const out: ActionItem[] = [
      { id: 'top', label: 'Move to top', icon: 'chevron-up', disabled: index === 0 },
      { id: 'up', label: 'Move up', icon: 'arrow-up', disabled: index === 0, hint: 'Alt+↑' },
      { id: 'down', label: 'Move down', icon: 'arrow-down', disabled: index === n - 1, hint: 'Alt+↓' },
      { id: 'bottom', label: 'Move to bottom', icon: 'chevron-down', disabled: index === n - 1 },
    ];
    if (groupable && !parent) {
      out.push({ id: 'group', label: 'Group with next', icon: 'link', group: 'g', disabled: index === n - 1 });
      if (it.children) out.push({ id: 'ungroup', label: 'Ungroup', icon: 'list', group: 'g' });
    }
    return out;
  }
  function onmenu(action: string, it: OrderedItem, parent: string | undefined, index: number) {
    const n = listOf(parent).length;
    if (action === 'top') move(it, parent, 0);
    else if (action === 'up') move(it, parent, index - 1);
    else if (action === 'down') move(it, parent, index + 1);
    else if (action === 'bottom') move(it, parent, n - 1);
    else if (action === 'group') {
      const below = items[index + 1];
      commit(groupWithNext(items, index));
      announcement = `${nameOf(it)} and ${below ? nameOf(below) : 'the next row'} now share one rank`;
    } else if (action === 'ungroup') {
      commit(ungroup(items, index));
      announcement = `${nameOf(it)} split into ${it.children?.length ?? 0} rows`;
    }
  }

  function toggle(it: OrderedItem, parent: string | undefined, on: boolean) {
    const next = clone(items);
    if (parent) {
      const g = next.find((x) => x.id === parent)!;
      g.children = g.children!.map((c) => (c.id === it.id ? { ...c, enabled: on } : c));
      g.enabled = g.children.some((c) => c.enabled !== false);
    } else {
      const i = next.findIndex((x) => x.id === it.id);
      next[i] = { ...next[i], enabled: on, children: next[i].children?.map((c) => (c.locked ? c : { ...c, enabled: on })) };
    }
    commit(next);
  }

  // Pointer drag (native drag and drop), within one list.
  function ondragstart(e: DragEvent, it: OrderedItem, parent: string | undefined, index: number) {
    if (readonly) return;
    drag = { id: it.id, parent, from: index };
    e.dataTransfer?.setData('text/plain', it.id);
    const row = (e.currentTarget as HTMLElement).closest('li');
    if (row && e.dataTransfer) { e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setDragImage(row, 16, 16); }
  }
  function ondragover(e: DragEvent, it: OrderedItem, parent: string | undefined) {
    if (!drag || drag.parent !== parent) return;
    e.preventDefault();
    e.stopPropagation();
    over = it.id;
  }
  function ondrop(e: DragEvent, parent: string | undefined, index: number) {
    if (!drag || drag.parent !== parent) return;
    e.preventDefault();
    e.stopPropagation();
    const moved = listOf(parent)[drag.from];
    commit(moveItem(items, parent, drag.from, index));
    if (moved) announcePos(moved, parent);
    drag = undefined;
    over = undefined;
  }
  function ondragend() { drag = undefined; over = undefined; }
</script>

{#snippet row(it: OrderedItem, parent: string | undefined, index: number, count: number)}
  {@const isGrabbed = grabbed?.id === it.id}
  <li
    class="item"
    class:group={!!it.children}
    class:grabbed={isGrabbed}
    class:over={over === it.id && drag?.id !== it.id}
    class:off={it.enabled === false}
    ondragover={(e) => ondragover(e, it, parent)}
    ondrop={(e) => ondrop(e, parent, index)}
  >
    <div class="line">
      {#if !readonly}
        <button
          type="button"
          class="handle"
          data-handle={it.id}
          draggable="true"
          aria-label="Reorder {it.label}, position {index + 1} of {count}"
          aria-describedby="{id}-help"
          aria-pressed={isGrabbed}
          onkeydown={(e) => onhandlekey(e, it, parent, index)}
          onblur={() => dropOnLeave(it.id)}
          ondragstart={(e) => ondragstart(e, it, parent, index)}
          {ondragend}
        ><Icon name="grip" size="sm" /></button>
      {/if}
      <span class="rank num" aria-hidden="true">{index + 1}</span>
      {#if toggleable}
        <Checkbox
          checked={it.enabled !== false}
          disabled={readonly || !!it.locked}
          aria-label="Allow {it.label}"
          onchange={(e) => toggle(it, parent, (e.currentTarget as HTMLInputElement).checked)}
        />
      {/if}
      <span class="text">
        <span class="label">{it.label}</span>
        {#if it.children}<span class="count">{it.children.length} share this rank</span>{/if}
        {#if it.detail}<span class="detail">{it.detail}</span>{/if}
        {#if it.locked}<span class="locked"><Icon name="lock" size="sm" />{it.locked}</span>{/if}
      </span>
      {#if !readonly}
        <span class="menu">
          <ActionMenu size="sm" label="Order actions for {it.label}" items={menu(it, parent, index)} onselect={(a) => onmenu(a, it, parent, index)} />
        </span>
      {/if}
    </div>
    {#if it.children}
      <ol class="children" aria-label="{it.label}, equal rank">
        {#each it.children as c, ci (c.id)}{@render row(c, it.id, ci, it.children.length)}{/each}
      </ol>
    {/if}
  </li>
{/snippet}

<div class="ole" class:compact={density === 'compact'} bind:this={root}>
  {#if hint}<p class="hint" id="{id}-hint">{hint}</p>{/if}
  <p class="visually-hidden" id="{id}-help">Space lifts the row, arrow keys move it, Space drops it, Escape cancels. Alt with an arrow key moves at once.</p>
  <ol class="list" aria-label={label} aria-describedby={hint ? `${id}-hint` : undefined}>
    {#each items as it, i (it.id)}{@render row(it, undefined, i, items.length)}{/each}
  </ol>
  <p class="visually-hidden" role="status" aria-live="polite">{announcement}</p>
</div>

<style>
  .ole { display: grid; gap: var(--space-2); min-width: 0; }
  .hint { font-size: var(--text-sm); color: var(--ink-muted); }
  .list, .children { list-style: none; margin: 0; padding: 0; display: grid; }
  .list { border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); background: var(--surface-raised); overflow: clip; }
  .item { border-top: var(--border-w) solid var(--line); background: var(--surface-raised); transition: background-color var(--dur-fast) var(--ease-out); }
  .list > .item:first-child { border-top: 0; }
  .item.over { background: var(--accent-tint); }
  .item.grabbed > .line { background: var(--surface-selected); box-shadow: var(--shadow-sm); }
  .line { display: flex; align-items: center; gap: var(--space-2); min-height: var(--row-h-comfortable); padding: 0 var(--space-2); min-width: 0; }
  .compact .line { min-height: var(--row-h-compact); }
  .children { margin: 0 var(--space-3) var(--space-2) var(--space-10); border: var(--border-w) dashed var(--line-strong); border-radius: var(--radius-md); }
  .children .item { background: transparent; }
  .children > .item:first-child { border-top: 0; }
  .handle {
    display: inline-grid; place-items: center; width: var(--control-h-sm); height: var(--control-h-sm); flex: none;
    border: 0; border-radius: var(--radius-sm); background: transparent; color: var(--ink-subtle); cursor: grab; position: relative;
  }
  .handle:hover { background: var(--surface-hover); color: var(--ink); }
  .handle[aria-pressed='true'] { background: var(--accent); color: var(--ink-on-accent); cursor: grabbing; }
  .rank { width: 1.5rem; text-align: end; font-size: var(--text-sm); color: var(--ink-subtle); flex: none; }
  .text { display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--space-0-5) var(--space-2); flex: 1; min-width: 0; padding: var(--space-1-5) 0; }
  .label { font-size: var(--text-md); color: var(--ink); overflow-wrap: anywhere; }
  .group > .line .label { font-weight: var(--weight-semibold); }
  .off > .line .label { color: var(--ink-muted); }
  .count, .detail { font-size: var(--text-xs); color: var(--ink-muted); }
  .locked { display: inline-flex; align-items: center; gap: var(--space-1); font-size: var(--text-xs); color: var(--ink-muted); }
  .menu { flex: none; }
  @media (width < 48rem), (pointer: coarse) {
    .handle { width: var(--target-min); height: var(--target-min); }
    .children { margin-left: var(--space-4); }
  }
</style>
