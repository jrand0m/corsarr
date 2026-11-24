<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import LibraryMusic from './LibraryMusic.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/Library Music', component: LibraryMusic, parameters: fullscreen });
</script>

<Story name="Artists desktop light" globals={desktopLight} />
<Story name="Artists desktop dark" globals={desktopDark} />
<Story name="Artists phone light" globals={phoneLight} />
<Story name="Artists phone dark" globals={phoneDark} />
<Story name="Selection with bulk bar" args={{ selected: ['a1', 'a3'] }} globals={desktopLight} />
<Story name="Empty" args={{ empty: true }} globals={desktopLight} />
