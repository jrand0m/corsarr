import Queue from '../screens/activity/Queue.svelte';
import History from '../screens/activity/History.svelte';
import Blocklist from '../screens/activity/Blocklist.svelte';
import type { AreaModule } from '../areas';

export const screens = { queue: Queue, history: History, blocklist: Blocklist } as unknown as AreaModule['screens'];
