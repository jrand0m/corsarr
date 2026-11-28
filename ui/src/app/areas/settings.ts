import General from '../screens/settings/General.svelte';
import Plugins from '../screens/settings/Plugins.svelte';
import PluginModules from '../screens/settings/PluginModules.svelte';
import PluginModule from '../screens/settings/PluginModule.svelte';
import PluginCatalogue from '../screens/settings/PluginCatalogue.svelte';
import Indexers from '../screens/settings/Indexers.svelte';
import type { AreaModule } from '../areas';

export const screens = {
  general: General, plugins: Plugins, modules: PluginModules, module: PluginModule, catalogue: PluginCatalogue, indexers: Indexers,
} as unknown as AreaModule['screens'];
