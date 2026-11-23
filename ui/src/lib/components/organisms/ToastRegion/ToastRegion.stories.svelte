<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ToastRegion from './ToastRegion.svelte';
  import Button from '../../atoms/Button/Button.svelte';
  import { ToastQueue, type ToastInput } from '../../../toast/toasts.svelte';
  import ThemePair from '../../../storybook/ThemePair.svelte';
  import Row from '../../../storybook/Row.svelte';

  const samples: ToastInput[] = [
    { message: 'Removed The Long Haul S02E03 and blocklisted the release', detail: 'Searching for another release.', onundo: () => {} },
    { message: 'Grab sent to qBittorrent', tone: 'info', actionLabel: 'Open queue', onaction: () => {} },
    { message: 'Held 3 downloads', detail: 'Rules leave them alone until released.', tone: 'warn', onundo: () => {} },
    { message: "Couldn't pause Dockside (2026)", detail: 'qBittorrent did not answer within 5 s.', tone: 'error' },
  ];
  const still = (t: ToastInput): ToastInput => ({ ...t, duration: 0 });
  function filled(n: number) {
    const q = new ToastQueue();
    samples.slice(0, n).forEach((t) => q.push(still(t)));
    return q;
  }
  const stacked = filled(3);
  const overflow = filled(4);
  const top = filled(1);

  const { Story } = defineMeta({
    title: 'Organisms/ToastRegion',
    component: ToastRegion,
    tags: ['autodocs'],
    args: { placement: 'bottom-end', max: 3 },
  });
</script>

<script lang="ts">
  const live = new ToastQueue();
  let n = 0;
  function push() {
    live.push(samples[n % samples.length]);
    n++;
  }
</script>

<Story name="Playground">
  {#snippet template(args)}
    <div class="stage">
      <Button onclick={push}>Push a toast</Button>
      <ToastRegion {...args} queue={live} />
    </div>
  {/snippet}
</Story>

<Story name="All states" asChild>
  <ThemePair>
      <Row label="Stacked, bottom-end"><div class="stage"><ToastRegion queue={stacked} /></div></Row>
      <Row label="Over the limit: the rest wait"><div class="stage"><ToastRegion queue={overflow} max={2} /></div></Row>
      <Row label="Top-center, one outcome"><div class="stage short"><ToastRegion queue={top} placement="top-center" /></div></Row>
  </ThemePair>
</Story>

<style>
  /* transform makes the stage the containing block of the fixed region (story only). */
  .stage { position: relative; transform: translateZ(0); width: 100%; min-height: 20rem; padding: var(--space-3); background: var(--surface-sunken); border-radius: var(--radius-lg); }
  .short { min-height: 9rem; }
</style>
