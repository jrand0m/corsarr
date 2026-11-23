<!--
  ToggleMatrix: rows × columns of toggles or selects. Notification events × connections
  with a priority column, device capabilities × policy, migration scope × kinds.

  One tab stop for the whole grid (roving focus): arrow keys, Home/End move between cells,
  Space toggles. Toggle columns get an "all rows" checkbox in the header. On phones each row
  becomes a stacked card with the column name beside every control.
-->
<script lang="ts" module>
  import type { SelectOption } from '../../atoms/Select/Select.svelte';

  export interface MatrixColumn {
    id: string;
    label: string;
    kind?: 'toggle' | 'select';
    /** select columns. */
    options?: SelectOption[];
  }
  export interface MatrixRow { id: string; label: string; description?: string; group?: string }
  export type MatrixCell = boolean | string;
  export type MatrixValue = Record<string, Record<string, MatrixCell>>;
</script>

<script lang="ts">
  import Checkbox from '../../atoms/Checkbox/Checkbox.svelte';
  import Select from '../../atoms/Select/Select.svelte';
  import Tooltip from '../../atoms/Tooltip/Tooltip.svelte';

  interface Props {
    rows: MatrixRow[];
    columns: MatrixColumn[];
    value?: MatrixValue;
    /** Accessible name: "Which connection hears which event". */
    label: string;
    /** Header of the row-label column: "Event". */
    rowHeader?: string;
    /** Why a cell is unavailable ("Telegram has no priority"); undefined when it is available. */
    unavailable?: (row: MatrixRow, column: MatrixColumn) => string | undefined;
    readonly?: boolean;
    /** Show "all rows" checkboxes in toggle column headers. */
    columnToggles?: boolean;
    onchange?: (value: MatrixValue, cell: { row: string; column: string }) => void;
  }
  let {
    rows, columns, value = $bindable({}), label, rowHeader = '', unavailable, readonly = false,
    columnToggles = true, onchange,
  }: Props = $props();

  let table = $state<HTMLTableElement>();
  let active = $state({ r: 0, c: 0 });

  const groups = $derived.by(() => {
    const out: { name?: string; rows: { row: MatrixRow; index: number }[] }[] = [];
    rows.forEach((row, index) => {
      const last = out[out.length - 1];
      if (last && last.name === row.group) last.rows.push({ row, index });
      else out.push({ name: row.group, rows: [{ row, index }] });
    });
    return out;
  });

  const cell = (r: string, c: string): MatrixCell | undefined => value[r]?.[c];
  function set(r: string, c: string, v: MatrixCell) {
    value = { ...value, [r]: { ...value[r], [c]: v } };
    onchange?.(value, { row: r, column: c });
  }
  function columnState(col: MatrixColumn) {
    const open = rows.filter((r) => !unavailable?.(r, col));
    const on = open.filter((r) => cell(r.id, col.id) === true).length;
    return { all: open.length > 0 && on === open.length, some: on > 0 && on < open.length, open };
  }
  function setColumn(col: MatrixColumn, on: boolean) {
    const next = { ...value };
    for (const r of columnState(col).open) next[r.id] = { ...next[r.id], [col.id]: on };
    value = next;
    onchange?.(value, { row: '*', column: col.id });
  }

  function focusCell(r: number, c: number) {
    active = { r, c };
    table?.querySelector<HTMLElement>(`[data-r="${r}"][data-c="${c}"] :is(input, select, [data-cell])`)?.focus();
  }
  function onkeydown(e: KeyboardEvent) {
    const t = (e.target as HTMLElement).closest<HTMLElement>('[data-r]');
    if (!t) return;
    const r = Number(t.dataset.r), c = Number(t.dataset.c);
    const isSelect = (e.target as HTMLElement).tagName === 'SELECT';
    const moves: Record<string, [number, number]> = {
      ArrowRight: [r, Math.min(columns.length - 1, c + 1)],
      ArrowLeft: [r, Math.max(0, c - 1)],
      ArrowDown: [Math.min(rows.length - 1, r + 1), c],
      ArrowUp: [Math.max(0, r - 1), c],
      Home: [e.ctrlKey ? 0 : r, 0],
      End: [e.ctrlKey ? rows.length - 1 : r, columns.length - 1],
    };
    // A select keeps up/down for its own options.
    if (isSelect && (e.key === 'ArrowDown' || e.key === 'ArrowUp')) return;
    const to = moves[e.key];
    if (!to) return;
    e.preventDefault();
    focusCell(to[0], to[1]);
  }
</script>

<div class="matrix-wrap">
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions (arrow keys from the cell controls bubble here: one delegated handler for the roving focus) -->
  <table class="matrix" aria-label={label} bind:this={table} {onkeydown}>
    <thead>
      <tr>
        <th scope="col" class="rowhead">{rowHeader}</th>
        {#each columns as col (col.id)}
          {@const st = columnState(col)}
          <th scope="col" class:sel={col.kind === 'select'}>
            <span class="colhead">
              <span>{col.label}</span>
              {#if columnToggles && col.kind !== 'select' && !readonly && rows.length > 1}
                <Checkbox checked={st.all} indeterminate={st.some} aria-label="All rows: {col.label}" tabindex={-1}
                  onchange={(e) => setColumn(col, (e.currentTarget as HTMLInputElement).checked)} />
              {/if}
            </span>
          </th>
        {/each}
      </tr>
    </thead>
    {#each groups as g, gi (gi)}
      <tbody>
        {#if g.name}
          <tr class="grouprow"><th scope="colgroup" colspan={columns.length + 1}>{g.name}</th></tr>
        {/if}
        {#each g.rows as { row, index: r } (row.id)}
          <tr>
            <th scope="row" class="rowhead">
              <span class="rl">{row.label}</span>
              {#if row.description}<span class="rd">{row.description}</span>{/if}
            </th>
            {#each columns as col, c (col.id)}
              {@const reason = unavailable?.(row, col)}
              {@const tab = active.r === r && active.c === c ? 0 : -1}
              <td data-r={r} data-c={c} data-col={col.label} class:sel={col.kind === 'select'}>
                {#if reason}
                  <Tooltip text={reason}>
                    {#snippet children(t)}
                      <span class="na" data-cell {...t} role="button" aria-disabled="true" aria-label="{col.label} for {row.label}: unavailable, {reason}" tabindex={tab} onfocusin={() => (active = { r, c })}>—</span>
                    {/snippet}
                  </Tooltip>
                {:else if col.kind === 'select'}
                  <Select
                    size="sm"
                    options={col.options}
                    value={String(cell(row.id, col.id) ?? '')}
                    disabled={readonly}
                    aria-label="{col.label} for {row.label}"
                    tabindex={tab}
                    onfocus={() => (active = { r, c })}
                    onchange={(e) => set(row.id, col.id, (e.currentTarget as HTMLSelectElement).value)}
                  />
                {:else}
                  <Checkbox
                    checked={cell(row.id, col.id) === true}
                    disabled={readonly}
                    aria-label="{col.label} for {row.label}"
                    tabindex={tab}
                    onfocus={() => (active = { r, c })}
                    onchange={(e) => set(row.id, col.id, (e.currentTarget as HTMLInputElement).checked)}
                  />
                {/if}
              </td>
            {/each}
          </tr>
        {/each}
      </tbody>
    {/each}
  </table>
</div>

<style>
  .matrix-wrap { container: matrix / inline-size; overflow-x: auto; border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); background: var(--surface-raised); }
  .matrix { border-collapse: collapse; width: 100%; font-size: var(--text-md); }
  th, td { padding: var(--space-1-5) var(--space-3); border-top: var(--border-w) solid var(--line); text-align: center; vertical-align: middle; }
  thead th { border-top: 0; background: var(--surface-sunken); font-size: var(--text-sm); font-weight: var(--weight-medium); color: var(--ink-muted); white-space: nowrap; }
  .colhead { display: inline-flex; align-items: center; gap: var(--space-1-5); }
  .rowhead { text-align: start; font-weight: var(--weight-regular); width: 40%; min-width: 12rem; }
  thead .rowhead { font-weight: var(--weight-medium); }
  .rl { color: var(--ink); }
  .rl, .rd { display: block; }
  .rd { font-size: var(--text-xs); color: var(--ink-muted); margin-top: var(--space-0-5); }
  td { height: var(--row-h-comfortable); }
  td.sel, th.sel { text-align: start; min-width: 8rem; }
  .grouprow th { text-align: start; font-size: var(--text-sm); font-weight: var(--weight-semibold); color: var(--ink); background: var(--surface-ground); }
  .na { display: inline-grid; place-items: center; min-width: var(--control-h-sm); min-height: var(--control-h-sm); color: var(--ink-subtle); border-radius: var(--radius-sm); }
  tbody tr:hover td, tbody tr:hover .rowhead { background: var(--surface-hover); }
  .grouprow:hover th { background: var(--surface-ground); }
  /* Narrow (a phone, or a narrow panel): each row becomes a stacked card. */
  @container matrix (width < 36rem) {
    .matrix, .matrix tbody, .matrix tr, .matrix th, .matrix td { display: block; width: auto; }
    .matrix thead { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); }
    tbody tr { padding: var(--space-2) var(--space-3); border-top: var(--border-w) solid var(--line); }
    tbody tr th, tbody tr td { border: 0; padding: 0; }
    tbody .rowhead { min-width: 0; padding-bottom: var(--space-1); }
    td { display: flex !important; align-items: center; justify-content: space-between; gap: var(--space-3); min-height: var(--target-min); height: auto; }
    td::before { content: attr(data-col); color: var(--ink-muted); font-size: var(--text-md); }
    .grouprow { padding: var(--space-2) var(--space-3); background: var(--surface-ground); }
    .grouprow th { background: transparent; }
  }
</style>
