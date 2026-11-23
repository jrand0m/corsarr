<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import DataTable, { type Column } from './DataTable.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import DataTableDemo from '../../../storybook/DataTableDemo.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import EmptyState from '../../molecules/EmptyState/EmptyState.svelte';
  import PropertyList from '../../molecules/PropertyList/PropertyList.svelte';
  import { libraryRows, type LibraryRow } from '../../../storybook/library-rows';

  const rows = libraryRows(40);
  const columns: Column<LibraryRow>[] = [
    { id: 'title', label: 'Title', value: (r) => r.title, phone: 'title' },
    { id: 'profile', label: 'Profile', width: '6rem', value: (r) => r.profile },
    { id: 'year', label: 'Year', width: '5rem', align: 'end', value: (r) => String(r.year) },
  ];
  const key = (r: LibraryRow) => r.id;

  const { Story } = defineMeta({ title: 'Organisms/DataTable', component: DataTable, tags: ['autodocs'] });
</script>

<Story name="Playground" asChild>
  <DataTableDemo />
</Story>

<Story name="Ten thousand rows, compact" asChild>
  <ThemePair>
    <Row label="Virtualised: the DOM holds about one viewport of rows">
      <div style="width: 100%"><DataTableDemo density="compact" mode="table" /></div>
    </Row>
  </ThemePair>
</Story>

<Story name="Phone list mode" asChild>
  <ThemePair>
    <Row label="Below 768 px the table becomes a list">
      <div style="width: 100%; max-width: 375px"><DataTableDemo mode="list" count={500} /></div>
    </Row>
  </ThemePair>
</Story>

<Story name="States" asChild>
  <ThemePair>
    <Row label="Loading"><div style="width: 100%"><DataTable rows={[]} {columns} rowKey={key} label="Library loading" loading height="16rem" /></div></Row>
    <Row label="Empty with next action">
      <div style="width: 100%">
        <DataTable rows={[]} {columns} rowKey={key} label="Library, empty" noun="series" nounPlural="series">
          {#snippet empty()}
            <EmptyState title="No series yet" icon="tv">Add a series to start tracking its episodes.{#snippet actions()}<Button variant="primary" icon="plus">Add series</Button>{/snippet}</EmptyState>
          {/snippet}
        </DataTable>
      </div>
    </Row>
    <Row label="Error"><div style="width: 100%"><DataTable rows={[]} {columns} rowKey={key} label="Library, failed" noun="series" nounPlural="series" error="The server did not answer in 10 s." errorCode="HTTP 504 · api.timeout" onretry={() => {}} /></div></Row>
    <Row label="Loading more"><div style="width: 100%"><DataTable rows={rows.slice(0, 6)} {columns} rowKey={key} label="Library, loading more" loadingMore hasMore total={412} height="18rem" /></div></Row>
    <Row label="Next page failed"><div style="width: 100%"><DataTable rows={rows.slice(0, 6)} {columns} rowKey={key} label="Library, page failed" error="Connection reset" onretry={() => {}} hasMore total={412} height="18rem" /></div></Row>
    <Row label="Selected rows with bulk actions">
      <div style="width: 100%">
        <DataTable rows={rows.slice(0, 8)} {columns} rowKey={key} label="Library, selection" selectable selected={['item-1', 'item-2', 'item-5']} total={412} height="18rem">
          {#snippet bulkActions(keys)}<Button size="sm" variant="primary" icon="search">Search {keys.length}</Button><Button size="sm" icon="trash">Delete…</Button>{/snippet}
        </DataTable>
      </div>
    </Row>
    <Row label="Current row (open in a drawer)"><div style="width: 100%"><DataTable rows={rows.slice(0, 5)} {columns} rowKey={key} label="Library, current" current="item-2" onopen={() => {}} height="14rem" /></div></Row>
  </ThemePair>
</Story>

<Story name="Expandable rows" asChild>
  <ThemePair>
    <Row label="One row expanded (table): detail sits under the row; Right and Left arrows open and close it">
      <div style="width: 100%">
        <DataTable rows={rows.slice(0, 12)} {columns} rowKey={key} label="Library, expandable" mode="table" expanded={['item-1']} height="22rem">
          {#snippet expand(r)}
            <PropertyList density="compact" label="Details for {r.title}" items={[
              { key: 'status', label: 'Status', value: r.status },
              { key: 'units', label: 'On disk', value: `${r.have} of ${r.total}` },
              { key: 'path', label: 'Folder', value: `/data/media/${r.kind}/${r.title} (${r.year})`, mono: true, copy: true },
            ]} />
          {/snippet}
        </DataTable>
      </div>
    </Row>
    <Row label="Phone list with an expanded row">
      <div style="width: 100%; max-width: 23.4375rem">
        <DataTable rows={rows.slice(0, 8)} {columns} rowKey={key} label="Library, expandable list" mode="list" expanded={['item-2']} height="24rem">
          {#snippet expand(r)}
            <PropertyList density="compact" layout="stacked" label="Details for {r.title}" items={[
              { key: 'units', label: 'On disk', value: `${r.have} of ${r.total}` },
              { key: 'path', label: 'Folder', value: `/data/media/${r.kind}/${r.title}`, mono: true },
            ]} />
          {/snippet}
        </DataTable>
      </div>
    </Row>
  </ThemePair>
</Story>
