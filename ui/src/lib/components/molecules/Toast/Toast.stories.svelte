<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import Toast from './Toast.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Molecules/Toast', component: Toast, tags: ['autodocs'], args: { message: 'Removed The Long Haul S02E03 and blocklisted the release', detail: 'Searching for another release.', tone: 'ok', duration: 0 } });
</script>

<Story name="Playground">
  {#snippet template(args)}<Toast {...args} onundo={() => {}} ondismiss={() => {}} />{/snippet}
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Outcome with undo"><Toast message="Removed The Long Haul S02E03 and blocklisted the release" detail="Searching for another release." onundo={() => {}} ondismiss={() => {}} duration={0} /></Row>
    <Row label="Info with follow-up action"><Toast tone="info" message="Grab sent to qBittorrent" actionLabel="Open queue" onaction={() => {}} ondismiss={() => {}} duration={0} /></Row>
    <Row label="Warning"><Toast tone="warn" message="Held 3 downloads" detail="Rules will not act on them until released." onundo={() => {}} ondismiss={() => {}} duration={0} /></Row>
    <Row label="Failure (alert)"><Toast tone="error" message="Couldn't pause Dockside (2026)" detail="qBittorrent did not answer within 5 s." ondismiss={() => {}} duration={0} /></Row>
    <Row label="Auto-dismiss after 10 s (pauses on hover or focus)"><Toast message="Unblocklisted Dockside.2026.1080p.WEB" onundo={() => {}} ondismiss={() => {}} /></Row>
  </ThemePair>
</Story>
