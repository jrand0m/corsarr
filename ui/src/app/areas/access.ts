import FirstRun from '../screens/access/FirstRun.svelte';
import SignIn from '../screens/access/SignIn.svelte';
import type { AreaModule } from '../areas';

export const screens = { setup: FirstRun, login: SignIn } as unknown as AreaModule['screens'];
