import Dashboard from '../screens/dashboard/Dashboard.svelte';
import type { AreaModule } from '../areas';

export const screens = { dashboard: Dashboard } as unknown as AreaModule['screens'];
