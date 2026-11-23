// Per-area code splitting: each area's screens load as their own chunk on first visit.
import type { Component } from 'svelte';
import type { Area } from './router';

export type AreaModule = { screens: Record<string, Component<Record<string, unknown>>> };

export const areaLoaders: Record<Area, () => Promise<AreaModule>> = {
  access: () => import('./areas/access'),
  dashboard: () => import('./areas/dashboard'),
  library: () => import('./areas/library'),
  wanted: () => import('./areas/wanted'),
  activity: () => import('./areas/activity'),
  system: () => import('./areas/system'),
  settings: () => import('./areas/settings'),
};
