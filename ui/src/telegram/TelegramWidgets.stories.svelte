<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import VariantPickerWidget from './VariantPickerWidget.svelte';
  import QueueListWidget from './QueueListWidget.svelte';
  import ThemePair from '../lib/storybook/ThemePair.svelte';
  import Row from '../lib/storybook/Row.svelte';
  import { queueListSample, variantPickerSample } from './samples';

  // Telegram widget templates: the page bodies `npm run build-telegram`
  // renders into src/telegram/dist/*.html, shown at a Telegram phone width (360 px).
  const { Story } = defineMeta({
    title: 'Templates/Telegram widgets',
    component: VariantPickerWidget,
    tags: ['autodocs'],
    args: { data: variantPickerSample },
  });
</script>

<Story name="Playground">
  {#snippet template(args)}<div class="phone"><VariantPickerWidget {...args} /></div>{/snippet}
</Story>

<Story name="Both widgets at Telegram width" asChild>
  <ThemePair>
    <Row label="Variant picker: auto-picks the default in 0:42">
      <div class="phone"><VariantPickerWidget data={variantPickerSample} /></div>
    </Row>
    <Row label="Queue cards: plain form actions per download">
      <div class="phone"><QueueListWidget data={queueListSample} /></div>
    </Row>
  </ThemePair>
</Story>

<Story name="Edge cases" asChild>
  <ThemePair>
    <Row label="Variant picker without auto-pick or cap">
      <div class="phone">
        <VariantPickerWidget data={{ ...variantPickerSample, title: undefined, cap: undefined, autoAcceptAt: undefined, defaultId: 'l2' }} />
      </div>
    </Row>
    <Row label="Queue with Pause and Kill only">
      <div class="phone"><QueueListWidget data={{ ...queueListSample, actions: ['pause', 'resume', 'kill'] }} /></div>
    </Row>
    <Row label="Empty queue">
      <div class="phone"><QueueListWidget data={{ ...queueListSample, downloads: [] }} /></div>
    </Row>
  </ThemePair>
</Story>

<style>
  .phone { width: 360px; max-width: 100%; background: var(--surface-ground); }
</style>
