import Health from '../screens/system/Health.svelte';
import Tasks from '../screens/system/Tasks.svelte';
import Migration from '../screens/system/Migration.svelte';
import type { AreaModule } from '../areas';

export const screens = { health: Health, tasks: Tasks, migration: Migration } as unknown as AreaModule['screens'];
