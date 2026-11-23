// Templates barrel: organisms arranged into page layouts, no data fetching. Routes wire data.
export { default as AppShell, shellNav, goKeys, kindPaths } from './AppShell/AppShell.svelte';
export type { NavArea, NavLink, ShellStatus, Connection } from './AppShell/AppShell.svelte';
export { default as ListPage } from './ListPage/ListPage.svelte';
export { default as DetailPage } from './DetailPage/DetailPage.svelte';
export { default as SettingsPage } from './SettingsPage/SettingsPage.svelte';
export type { SettingsSection } from './SettingsPage/SettingsPage.svelte';
export { default as WizardPage } from './WizardPage/WizardPage.svelte';
export { default as AccessPage } from './AccessPage/AccessPage.svelte';
export { default as DashboardPage } from './DashboardPage/DashboardPage.svelte';
export type { AttentionItem, HealthProblem, DashboardRegion } from './DashboardPage/DashboardPage.svelte';
