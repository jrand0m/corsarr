import General from '../screens/settings/General.svelte';
import Plugins from '../screens/settings/Plugins.svelte';
import Indexers from '../screens/settings/Indexers.svelte';
import Profiles from '../screens/settings/Profiles.svelte';
import CustomFormats from '../screens/settings/CustomFormats.svelte';
import type { AreaModule } from '../areas';

export const screens = { general: General, plugins: Plugins, indexers: Indexers, profiles: Profiles, 'custom-formats': CustomFormats } as unknown as AreaModule['screens'];
