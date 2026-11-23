<!--
  VariantPickerWidget: the Telegram page body for the variant picker. Names the download being replaced, then
  the VariantPicker as a plain form (radios plus named submit buttons) posting to `formAction`.
  Works without scripts: the server picks the default at `autoAcceptAt` whether or not the page
  posts. The optional page script (build.mjs) only ticks the countdown and posts the default.
-->
<script lang="ts">
  import VariantPicker from '../lib/components/organisms/VariantPicker/VariantPicker.svelte';
  import MediaKindIcon from '../lib/components/atoms/MediaKindIcon/MediaKindIcon.svelte';
  import type { VariantPickerWidgetData } from './types';

  interface Props {
    data: VariantPickerWidgetData;
  }
  let { data }: Props = $props();
  const s = $derived(data.subject);
</script>

<div class="tg">
  <header class="subject">
    <span class="kind"><MediaKindIcon kind={s.kind} /></span>
    <div class="names">
      <h2>{s.title}{#if s.subtitle}<span class="sub">{` · ${s.subtitle}`}</span>{/if}</h2>
      {#if s.current}<p class="current">Now: {s.current}</p>{/if}
    </div>
  </header>
  <VariantPicker
    variants={data.variants}
    defaultId={data.defaultId}
    autoAcceptAt={data.autoAcceptAt}
    cap={data.cap}
    title={data.title}
    now={data.now}
    formAction={data.formAction}
  />
</div>

<style>
  .tg { display: grid; gap: var(--space-3); width: 100%; max-width: 30rem; margin-inline: auto; padding: var(--space-3); }
  .subject { display: flex; gap: var(--space-2); align-items: flex-start; min-width: 0; }
  .kind { display: grid; place-items: center; width: var(--control-h-sm); height: var(--control-h-sm); flex: none; background: var(--surface-sunken); border-radius: var(--radius-md); }
  .names { display: grid; gap: var(--space-0-5); min-width: 0; }
  h2 { font-size: var(--text-xl); overflow-wrap: anywhere; }
  .sub { font-weight: var(--weight-regular); color: var(--ink-muted); }
  .current { font-size: var(--text-sm); color: var(--ink-muted); }
  /* Without scripts the `chosen` class stays on the server's pick: follow the checked radio. */
  .tg :global(.opt:has(input:checked)) { background: var(--surface-selected); border-color: var(--accent); }
  .tg :global(.opt.chosen:not(:has(input:checked))) { border-color: var(--line); }
  .tg :global(.opt.chosen:not(:has(input:checked)):not(:hover)) { background: none; }
</style>
