<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ErrorState from './ErrorState.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Molecules/ErrorState', component: ErrorState, tags: ['autodocs'], args: { title: "Couldn't load the queue", code: 'HTTP 503 · downloader.unreachable', healthHref: '#health', retrying: false } });
</script>

<Story name="Playground">
  {#snippet template(args)}<ErrorState {...args} onretry={() => {}}>qBittorrent at 10.0.0.20:8080 did not answer.</ErrorState>{/snippet}
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Integration failure, retry and health"><ErrorState title="Couldn't load the queue" code="HTTP 503 · downloader.unreachable" healthHref="#health" onretry={() => {}}>qBittorrent at 10.0.0.20:8080 did not answer.</ErrorState></Row>
    <Row label="Retrying"><ErrorState title="Couldn't load history" code="HTTP 500" retrying onretry={() => {}} /></Row>
    <Row label="Small, partial section"><ErrorState size="sm" title="Indexer stats unavailable" code="stats.timeout" onretry={() => {}} /></Row>
  </ThemePair>
</Story>
