<!-- System › About: the running build and where it came from, the same fields as `corsarr version --json`. -->
<script lang="ts">
  import { onMount } from 'svelte';
  import Link from '../../../lib/components/atoms/Link/Link.svelte';
  import Skeleton from '../../../lib/components/atoms/Skeleton/Skeleton.svelte';
  import ErrorState from '../../../lib/components/molecules/ErrorState/ErrorState.svelte';
  import PropertyList from '../../../lib/components/molecules/PropertyList/PropertyList.svelte';
  import { app } from '../../context.svelte';
  import { buildProperties } from '../../build';

  const { status } = app();
  onMount(() => {
    if (!status.data) void status.load();
  });
  const items = $derived(status.data ? buildProperties(status.data.build ?? { version: status.data.version }) : []);
</script>

<h1 class="title">About</h1>
<p class="lede">corsarr manages TV, movies, music and books from one small binary.</p>

<section aria-labelledby="about-build">
  <h2 id="about-build">This build</h2>
  {#if status.error && !status.data}
    <ErrorState title="Couldn't load the build details" code={status.error.label} onretry={status.retry} level={3} />
  {:else if !status.data}
    <Skeleton shape="block" height="10rem" />
  {:else}
    <PropertyList label="Build details" {items} />
  {/if}
  <p class="note">
    Every release can be rebuilt from its tag and compared byte for byte with the published files.
    <Link external href="https://github.com/jrand0m/corsarr/blob/master/VERIFYING.md">How to verify a release</Link>
  </p>
</section>

<style>
  .title { font-size: var(--text-xl); margin-bottom: var(--space-2); }
  .lede { color: var(--ink-muted); margin-bottom: var(--space-6); }
  h2 { font-size: var(--text-lg); margin-bottom: var(--space-3); }
  section { max-width: 48rem; }
  .note { margin-top: var(--space-4); font-size: var(--text-sm); color: var(--ink-muted); }
</style>
