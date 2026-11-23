<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import FirstRun from './FirstRun.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/First run', component: FirstRun, parameters: fullscreen });
  const typed = { value: 'Quay-lights-7-after-dark', confirm: 'Quay-lights-7-after-dusk' };
</script>

<Story name="Desktop light" globals={desktopLight} />
<Story name="Desktop dark" globals={desktopDark} />
<Story name="Phone light" globals={phoneLight} />
<Story name="Phone dark" globals={phoneDark} />
<Story name="Confirm does not match" args={typed} globals={desktopLight} />
<Story name="Submitted empty, phone" args={{ showErrors: true }} globals={phoneDark} />
<Story name="Saving" args={{ value: typed.value, confirm: typed.value, saving: true }} globals={desktopDark} />
