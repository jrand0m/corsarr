<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import ThemePair from '../storybook/ThemePair.svelte';
  import Button from '../components/atoms/Button/Button.svelte';

  const { Story } = defineMeta({ title: 'Foundations/Motion', tags: ['autodocs'] });
  const durations = [
    ['dur-instant', '80 ms', 'press feedback'],
    ['dur-fast', '140 ms', 'hover, colour, tooltip'],
    ['dur-base', '200 ms', 'switch thumb, disclosure, toast in'],
    ['dur-slow', '280 ms', 'drawer, progress fill'],
  ];
</script>

<script lang="ts">
  let moved = $state(false);
</script>

<Story name="Durations and easing" asChild>
  <ThemePair>
    <Button onclick={() => (moved = !moved)}>Play</Button>
    {#each durations as [t, ms, use] (t)}
      <div style="display: grid; gap: var(--space-1)">
        <span style="font-size: var(--text-xs); color: var(--ink-muted)">--{t} · {ms} · {use} · ease-out</span>
        <div style="height: 0.75rem; background: var(--surface-sunken); border-radius: var(--radius-xs); position: relative">
          <span style="position: absolute; top: 0; left: 0; width: 2rem; height: 100%; background: var(--accent); border-radius: var(--radius-xs); transition: transform var(--{t}) var(--ease-out); transform: translateX({moved ? 'calc(var(--measure) / 3)' : '0'})"></span>
        </div>
      </div>
    {/each}
    <p style="font-size: var(--text-sm); color: var(--ink-muted)">Motion conveys state only. Under prefers-reduced-motion all durations are 0; countdowns stop ticking by the second, spinners and sheens hold still.</p>
  </ThemePair>
</Story>
