<!--
  Icon: one glyph from the authored set (icons.ts). Decorative by default (aria-hidden);
  pass `label` when the icon is the only carrier of meaning.
-->
<script lang="ts">
  import type { SVGAttributes } from 'svelte/elements';
  import { icons, type IconName } from './icons';
  import type { Size } from '../../../tokens';

  interface Props extends Omit<SVGAttributes<SVGSVGElement>, 'name'> {
    name: IconName;
    size?: Size;
    label?: string;
  }
  let { name, size = 'md', label, class: cls, ...rest }: Props = $props();
</script>

<svg
  class={['icon', size, cls]}
  viewBox="0 0 24 24"
  role={label ? 'img' : undefined}
  aria-label={label}
  aria-hidden={label ? undefined : 'true'}
  focusable="false"
  {...rest}
><path d={icons[name]} /></svg>

<style>
  .icon {
    width: var(--icon-md);
    height: var(--icon-md);
    fill: none;
    stroke: currentColor;
    stroke-width: 1.75;
    stroke-linecap: round;
    stroke-linejoin: round;
  }
  .sm { width: var(--icon-sm); height: var(--icon-sm); stroke-width: 2; }
  .lg { width: var(--icon-lg); height: var(--icon-lg); }
</style>
