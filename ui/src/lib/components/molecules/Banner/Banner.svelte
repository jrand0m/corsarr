<!--
  Banner: a page- or region-wide condition the owner should know while working: live
  updates disconnected, data stale, a kind in shadow mode, an object managed by config, a
  restart needed, an unsaved preview. `preset` fills tone, icon and title for those six; any
  of them can be overridden. Full border and tint with a leading icon (never a side bar).
  `live` announces a banner that appears mid-session (disconnect), politely.
-->
<script lang="ts" module>
  import type { IconName } from '../../atoms/Icon/icons';
  import type { Tone } from '../../../tokens';

  export type BannerPreset = 'disconnected' | 'stale' | 'shadow' | 'managed' | 'restart' | 'preview';
  export const bannerPresets: Record<BannerPreset, { tone: Tone; icon: IconName; title: string }> = {
    disconnected: { tone: 'warn', icon: 'plug', title: 'Live updates paused' },
    stale: { tone: 'warn', icon: 'clock', title: 'This may be out of date' },
    shadow: { tone: 'info', icon: 'shadow', title: 'Shadow mode' },
    managed: { tone: 'managed', icon: 'lock', title: 'Managed by config' },
    restart: { tone: 'warn', icon: 'refresh', title: 'Restart needed' },
    preview: { tone: 'info', icon: 'eye', title: 'Preview' },
  };
</script>

<script lang="ts">
  import type { Snippet } from 'svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';
  import IconButton from '../../atoms/IconButton/IconButton.svelte';

  interface Props {
    preset?: BannerPreset;
    tone?: Tone;
    icon?: IconName;
    title?: string;
    children?: Snippet;
    actions?: Snippet;
    ondismiss?: () => void;
    live?: boolean;
  }
  let { preset, tone, icon, title, children, actions, ondismiss, live = false }: Props = $props();

  const p = $derived(preset ? bannerPresets[preset] : undefined);
  const t = $derived(tone ?? p?.tone ?? 'info');
  const glyph = $derived(icon ?? p?.icon ?? 'info');
  const heading = $derived(title ?? p?.title);
</script>

<div class={['banner', t]} role={live ? 'status' : undefined}>
  <Icon name={glyph} size="md" />
  <div class="text">
    {#if heading}<strong class="title">{heading}</strong>{/if}
    {#if children}<span class="msg">{@render children()}</span>{/if}
  </div>
  {#if actions}<div class="actions">{@render actions()}</div>{/if}
  {#if ondismiss}<IconButton icon="x" label="Dismiss" size="sm" onclick={ondismiss} />{/if}
</div>

<style>
  .banner {
    --ink: var(--tone-info-ink);
    --tint: var(--tone-info-tint);
    --line: var(--tone-info-line);
    display: flex;
    align-items: flex-start;
    gap: var(--space-2) var(--space-3);
    padding: var(--space-2) var(--space-3);
    color: var(--ink);
    background: var(--tint);
    border: var(--border-w) solid var(--line);
    border-radius: var(--radius-md);
  }
  .banner > :global(svg) { margin-top: 0.125rem; flex: none; }
  .ok { --ink: var(--tone-ok-ink); --tint: var(--tone-ok-tint); --line: var(--tone-ok-line); }
  .warn { --ink: var(--tone-warn-ink); --tint: var(--tone-warn-tint); --line: var(--tone-warn-line); }
  .error { --ink: var(--tone-error-ink); --tint: var(--tone-error-tint); --line: var(--tone-error-line); }
  .neutral { --ink: var(--tone-neutral-ink); --tint: var(--tone-neutral-tint); --line: var(--tone-neutral-line); }
  .managed { --ink: var(--tone-managed-ink); --tint: var(--tone-managed-tint); --line: var(--tone-managed-line); }
  .text { display: flex; flex-wrap: wrap; gap: var(--space-0-5) var(--space-2); flex: 1; min-width: 0; font-size: var(--text-md); line-height: var(--leading-snug); padding-block: var(--space-0-5); }
  .title { font-weight: var(--weight-semibold); }
  .msg { color: var(--ink); max-width: var(--measure); }
  .actions { display: flex; flex-wrap: wrap; gap: var(--space-2); align-items: center; }
  @media (width < 48rem) {
    .banner { flex-wrap: wrap; }
    .actions { flex-basis: 100%; padding-left: calc(var(--icon-md) + var(--space-3)); }
  }
</style>
