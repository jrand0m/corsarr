<!-- "Managed by config" with where config comes from and how to change it. -->
<script lang="ts">
  import Banner from '../../../lib/components/molecules/Banner/Banner.svelte';
  import Button from '../../../lib/components/atoms/Button/Button.svelte';
  import { changeHow, sourceName } from './sections';

  let { kind, path, exportHref = '#/settings/plugins' }: { kind?: string; path?: string; exportHref?: string } = $props();
</script>

<Banner preset="managed" title="Managed by config">
  These settings come from {sourceName(kind, path)} and are read-only here. {changeHow(kind, path)}
  {#snippet actions()}<Button size="sm" icon="download" href={exportHref}>Export config</Button>{/snippet}
</Banner>
