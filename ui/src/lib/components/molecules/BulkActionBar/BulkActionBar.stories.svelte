<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import BulkActionBar from './BulkActionBar.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  const { Story } = defineMeta({ title: 'Molecules/BulkActionBar', component: BulkActionBar, tags: ['autodocs'], args: { selected: 3, loaded: 100, total: 412, noun: 'series', nounPlural: 'series' } });
</script>

<Story name="Playground">
  {#snippet template(args)}
    <BulkActionBar {...args} onselectall={() => {}} onclear={() => {}}>
      {#snippet actions()}<Button size="sm" variant="primary" icon="search">Search 3</Button><Button size="sm" icon="pencil">Edit</Button>{/snippet}
    </BulkActionBar>
  {/snippet}
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Some selected, more loaded">
      <BulkActionBar selected={3} loaded={100} total={412} noun="series" nounPlural="series" onselectall={() => {}} onclear={() => {}}>
        {#snippet actions()}<Button size="sm" variant="primary" icon="search">Search 3</Button><Button size="sm" icon="pencil">Edit</Button><Button size="sm" icon="trash">Delete…</Button>{/snippet}
      </BulkActionBar>
    </Row>
    <Row label="All loaded selected, more on the server">
      <BulkActionBar selected={100} loaded={100} total={412} noun="series" nounPlural="series" onselectall={() => {}} onclear={() => {}}>
        {#snippet actions()}<Button size="sm" icon="pause">Pause</Button>{/snippet}
      </BulkActionBar>
    </Row>
    <Row label="One selected">
      <BulkActionBar selected={1} loaded={8} noun="download" onselectall={() => {}} onclear={() => {}}>
        {#snippet actions()}<Button size="sm" icon="pause">Pause</Button><Button size="sm" icon="trash">Kill…</Button>{/snippet}
      </BulkActionBar>
    </Row>
    <Row label="Nothing selected renders nothing"><BulkActionBar selected={0} loaded={8} /></Row>
  </ThemePair>
</Story>
