<script module lang="ts">
  import { defineMeta } from '@storybook/addon-svelte-csf';
  import SignIn from './SignIn.svelte';
  import { fullscreen, desktopLight, desktopDark, phoneLight, phoneDark } from './story-params';

  const { Story } = defineMeta({ title: 'Pages/Sign in', component: SignIn, parameters: fullscreen });
</script>

<Story name="Desktop light" globals={desktopLight} />
<Story name="Desktop dark" globals={desktopDark} />
<Story name="Phone light" globals={phoneLight} />
<Story name="Phone dark" globals={phoneDark} />
<Story name="Wrong password" args={{ state: 'wrong' }} globals={desktopLight} />
<Story name="Wait after repeated failures, phone" args={{ state: 'wait' }} globals={phoneDark} />
<Story name="Session expired, reverse proxy notice" args={{ expired: true, proxy: true }} globals={desktopDark} />
