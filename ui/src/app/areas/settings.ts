import General from '../screens/settings/General.svelte';
import Plugins from '../screens/settings/Plugins.svelte';
import PluginModules from '../screens/settings/PluginModules.svelte';
import PluginModule from '../screens/settings/PluginModule.svelte';
import PluginCatalogue from '../screens/settings/PluginCatalogue.svelte';
import Indexers from '../screens/settings/Indexers.svelte';
import Profiles from '../screens/settings/Profiles.svelte';
import CustomFormats from '../screens/settings/CustomFormats.svelte';
import type { AreaModule } from '../areas';

export const screens = {
  general: General, plugins: Plugins, modules: PluginModules, module: PluginModule, catalogue: PluginCatalogue, indexers: Indexers,
  profiles: Profiles, 'custom-formats': CustomFormats,
} as unknown as AreaModule['screens'];
