<!--
  DesirabilityGauge: how close a release is to what the quality profile aims for, from both
  sides. A score out of 100, a word for the side (above, on or below target) with an arrow, and
  a track with the target in the middle and a marker pushed left (below) or right (above) by the
  distance. `parts` lists the reasons in plain words. No desirability means the quality is outside
  the profile.
-->
<script lang="ts" module>
  import { formatBytes } from '../../../format/quantity';

  export type Position = 'above' | 'on_target' | 'below';
  export interface Desirability {
    score: number;
    position: Position;
    qualitySteps: number;
    cutoff: string;
    sizeDelta?: number;
    preferredSize?: number;
    formatShortfall?: number;
    formatTarget?: number;
  }
  export const positionLabel: Record<Position, string> = { above: 'Above target', on_target: 'On target', below: 'Below target' };

  const steps = (n: number) => `${Math.abs(n)} ${Math.abs(n) === 1 ? 'step' : 'steps'}`;

  /** The desirability's reasons in plain words. */
  export function desirabilityParts(d: Desirability): string[] {
    const out = [d.qualitySteps === 0 ? `At the cutoff quality (${d.cutoff})`
      : `${steps(d.qualitySteps)} ${d.qualitySteps > 0 ? 'above' : 'below'} the cutoff quality (${d.cutoff})`];
    if (d.sizeDelta === undefined || !d.preferredSize) out.push('Size unknown');
    else if (Math.abs(d.sizeDelta) <= 0.1) out.push(`Near the preferred size (${formatBytes(d.preferredSize)})`);
    else out.push(`${Math.round(Math.abs(d.sizeDelta) * 100)}% ${d.sizeDelta > 0 ? 'larger' : 'smaller'} than the preferred size (${formatBytes(d.preferredSize)})`);
    if (d.formatShortfall) out.push(`Custom formats ${d.formatShortfall} points below the target of ${d.formatTarget ?? 0}`);
    return out;
  }
</script>

<script lang="ts">
  import Icon from '../../atoms/Icon/Icon.svelte';

  interface Props {
    desirability?: Desirability;
    /** Show the plain-word parts under the headline. */
    parts?: boolean;
  }
  let { desirability: d, parts = false }: Props = $props();

  const offset = $derived(d ? ((100 - d.score) / 2) * (d.position === 'above' ? 1 : d.position === 'below' ? -1 : 0) : 0);
  const icon = $derived(d?.position === 'above' ? 'arrow-up' : d?.position === 'below' ? 'arrow-down' : 'target');
</script>

<span class={['gauge', d?.position ?? 'outside']}>
  {#if d}
    <span class="head">
      <Icon name={icon} size="sm" />
      <span class="word">{positionLabel[d.position]}</span>
      <span class="score num" title="Closeness to the profile, out of 100">{d.score}<span class="of">/100</span></span>
      <span class="track" aria-hidden="true"><span class="target"></span><span class="mark" style:left="calc(50% + {offset}%)"></span></span>
    </span>
    {#if parts}
      <ul class="parts">{#each desirabilityParts(d) as p (p)}<li>{p}</li>{/each}</ul>
    {/if}
  {:else}
    <span class="head"><Icon name="minus-circle" size="sm" /><span class="word">Outside the profile</span></span>
  {/if}
</span>

<style>
  .gauge { display: inline-grid; gap: var(--space-1); font-size: var(--text-sm); color: var(--ink-muted); }
  .head { display: inline-flex; align-items: center; gap: var(--space-1-5); }
  .word { font-weight: var(--weight-medium); color: var(--ink); }
  .outside .word { color: var(--ink-muted); }
  .score { font-weight: var(--weight-medium); color: var(--ink); font-variant-numeric: tabular-nums; }
  .of { color: var(--ink-subtle); font-weight: var(--weight-regular); }
  .track { position: relative; inline-size: 4rem; block-size: 0.5rem; background: var(--surface-sunken); box-shadow: inset 0 0 0 1px var(--line); border-radius: var(--radius-xs); }
  .target { position: absolute; left: 50%; top: -2px; bottom: -2px; inline-size: 1px; background: var(--ink-subtle); }
  .mark { position: absolute; top: 50%; inline-size: 0.5rem; block-size: 0.5rem; translate: -50% -50%; background: var(--ink); border-radius: 50%; }
  .parts { display: grid; gap: var(--space-0-5); margin: 0; padding-left: var(--space-5); }
</style>
