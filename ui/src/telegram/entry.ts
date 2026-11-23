// Server-render entry for build.mjs: renders each Telegram widget with its sample data.
import type { Component } from 'svelte';
import { render } from 'svelte/server';
import VariantPickerWidget from './VariantPickerWidget.svelte';
import QueueListWidget from './QueueListWidget.svelte';
import { queueListSample, variantPickerSample } from './samples';

export { buildPage, extractStyles, markCountdowns } from './inline';

export interface RenderedWidget {
  file: string;
  title: string;
  /** Render time (ms) of the sample, for countdown offsets. */
  now: number;
  head: string;
  body: string;
}

// render() exposes head and body as getters, so they are read explicitly (no spread).
function page<P extends Record<string, unknown>>(component: Component<P>, props: P) {
  const r = render(component, { props });
  return { head: r.head, body: r.body };
}

export function renderAll(): RenderedWidget[] {
  return [
    {
      file: 'variant-picker.html',
      title: 'Pick a replacement release',
      now: variantPickerSample.now,
      ...page(VariantPickerWidget, { data: variantPickerSample }),
    },
    { file: 'queue-cards.html', title: 'Queue', now: queueListSample.now, ...page(QueueListWidget, { data: queueListSample }) },
  ];
}
