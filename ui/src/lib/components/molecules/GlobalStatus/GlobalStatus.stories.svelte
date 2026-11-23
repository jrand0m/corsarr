<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import GlobalStatus from './GlobalStatus.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const { Story } = defineMeta({ title: 'Molecules/GlobalStatus', component: GlobalStatus, tags: ['autodocs'], args: { unhealthy: 2, healthTone: 'error', active: 4, blocked: 1, connection: 'live', shadowKinds: ['Music'] } });
</script>

<Story name="Playground">
  {#snippet template(args)}<div class="shell"><GlobalStatus {...args} /></div>{/snippet}
</Story>

<Story name="All states" asChild>
  <ThemePair>
    <Row label="Healthy, live"><div class="shell"><GlobalStatus unhealthy={0} active={2} blocked={0} connection="live" /></div></Row>
    <Row label="Problems, blocked, shadow"><div class="shell"><GlobalStatus unhealthy={2} healthTone="error" active={4} blocked={1} connection="live" shadowKinds={['Music', 'Books']} /></div></Row>
    <Row label="Reconnecting"><div class="shell"><GlobalStatus unhealthy={1} active={4} blocked={0} connection="reconnecting" /></div></Row>
    <Row label="Disconnected, compact (phone top bar)"><div class="shell"><GlobalStatus unhealthy={1} active={0} blocked={2} connection="offline" compact /></div></Row>
    <Row label="Vertical (side rail)"><div class="shell rail"><GlobalStatus unhealthy={0} active={3} blocked={0} connection="live" shadowKinds={['Music']} orientation="vertical" /></div></Row>
  </ThemePair>
</Story>

<style>
  .shell { padding: var(--space-2); background: var(--shell-bg); color: var(--shell-ink); border-radius: var(--radius-md); }
  .rail { width: var(--rail-w); }
</style>
