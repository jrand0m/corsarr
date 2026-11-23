<!--
  Skeleton (atom): a layout-stable placeholder while data loads. Compose skeletons in the
  exact shape of the content they stand in for; mark the loading region aria-busy="true".
  Purely decorative (aria-hidden). The sheen stops under reduced motion.
-->
<script lang="ts">
  interface Props {
    shape?: 'text' | 'block' | 'circle';
    width?: string;
    height?: string;
    /** text: number of lines; the last is shorter. */
    lines?: number;
  }
  let { shape = 'text', width = '100%', height, lines = 1 }: Props = $props();
</script>

{#if shape === 'text'}
  <span class="stack" style:width aria-hidden="true">
    {#each { length: lines } as _, i (i)}
      <span class="sk line" style:width={lines > 1 && i === lines - 1 ? '62%' : '100%'}></span>
    {/each}
  </span>
{:else}
  <span class={['sk', shape]} style:width style:height={height ?? width} aria-hidden="true"></span>
{/if}

<style>
  .stack { display: inline-grid; gap: 0.5em; vertical-align: middle; }
  .sk {
    display: block;
    background: linear-gradient(90deg, var(--surface-sunken) 0 40%, var(--surface-hover) 50%, var(--surface-sunken) 60% 100%) 0 0 / 300% 100%;
    border-radius: var(--radius-xs);
    animation: sheen 1.6s var(--ease-linear) infinite;
  }
  .line { height: 0.75em; }
  .block { border-radius: var(--radius-md); }
  .circle { border-radius: var(--radius-round); }
  @keyframes sheen { from { background-position: 100% 0; } to { background-position: 0 0; } }
  @media (prefers-reduced-motion: reduce) { .sk { animation: none; background: var(--surface-sunken); } }
</style>
