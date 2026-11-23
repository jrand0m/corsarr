import General from '../screens/settings/General.svelte';
import Plugins from '../screens/settings/Plugins.svelte';
import type { AreaModule } from '../areas';

export const screens = { general: General, plugins: Plugins } as unknown as AreaModule['screens'];
