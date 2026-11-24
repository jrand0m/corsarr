import Missing from '../screens/wanted/Missing.svelte';
import MissingEpisodes from '../screens/wanted/MissingEpisodes.svelte';
import MissingAlbums from '../screens/wanted/MissingAlbums.svelte';
import type { AreaModule } from '../areas';

export const screens = { missing: Missing, missingTv: MissingEpisodes, missingMusic: MissingAlbums } as unknown as AreaModule['screens'];
