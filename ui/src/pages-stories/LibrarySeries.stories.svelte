<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import LibrarySeries from './LibrarySeries.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/Library Series', component: LibrarySeries, parameters: fullscreen });
</script>

<Story name="Table desktop light" globals={desktopLight} />
<Story name="Table desktop dark" globals={desktopDark} />
<Story name="Table phone light" globals={phoneLight} />
<Story name="Table phone dark" globals={phoneDark} />
<Story name="Posters desktop light" args={{ view: 'posters' }} globals={desktopLight} />
<Story name="Posters phone dark" args={{ view: 'posters' }} globals={phoneDark} />
<Story name="Selection with bulk bar" args={{ selected: ['s1', 's3'] }} globals={desktopLight} />
<Story name="Empty" args={{ empty: true }} globals={desktopLight} />
