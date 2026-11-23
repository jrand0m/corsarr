<!--
  Story helper for Pages/*: the AppShell as the app mounts it, with fixture status, kinds and
  palette items. Pages pass the route path and their content. Not a design-system component.
-->
<script lang="ts">
  import type { ComponentProps, Snippet } from 'svelte';
  import AppShell from '../lib/components/templates/AppShell/AppShell.svelte';
  import { ToastQueue } from '../lib/toast/toasts.svelte';
  import { kinds, paletteItems, shellStatus } from '../lib/storybook/fixtures';

  interface Props extends Partial<Omit<ComponentProps<typeof AppShell>, 'children'>> {
    path: string;
    toasts?: ToastQueue;
    children: Snippet;
  }
  let { path, toasts = new ToastQueue(), children, ...rest }: Props = $props();
</script>

<AppShell {kinds} {path} status={shellStatus} {paletteItems} {toasts} onsignout={() => {}} onnavigate={() => {}} {...rest}>
  {@render children()}
</AppShell>
