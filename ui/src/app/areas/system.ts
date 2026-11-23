import Health from '../screens/system/Health.svelte';
import Tasks from '../screens/system/Tasks.svelte';
import type { AreaModule } from '../areas';

export const screens = { health: Health, tasks: Tasks } as unknown as AreaModule['screens'];
