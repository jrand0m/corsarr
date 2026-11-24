import General from '../screens/settings/General.svelte';
import Plugins from '../screens/settings/Plugins.svelte';
import Indexers from '../screens/settings/Indexers.svelte';
import type { AreaModule } from '../areas';

export const screens = { general: General, plugins: Plugins, indexers: Indexers } as unknown as AreaModule['screens'];
