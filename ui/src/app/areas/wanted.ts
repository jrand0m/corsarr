import Missing from '../screens/wanted/Missing.svelte';
import MissingEpisodes from '../screens/wanted/MissingEpisodes.svelte';
import type { AreaModule } from '../areas';

export const screens = { missing: Missing, missingTv: MissingEpisodes } as unknown as AreaModule['screens'];
