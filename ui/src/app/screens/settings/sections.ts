// Settings sections the web UI ships today; the rest arrive in later releases.
import type { SettingsSection } from '../../../lib/components/templates/SettingsPage/SettingsPage.svelte';

export const settingsSections = (instances?: number): SettingsSection[] => [
  { id: 'general', label: 'General', href: '#/settings/general' },
  { id: 'plugins', label: 'Plugins & integrations', href: '#/settings/plugins', count: instances },
  { id: 'indexers', label: 'Indexers', href: '#/settings/indexers' },
  { id: 'profiles', label: 'Profiles', href: '#/settings/profiles' },
  { id: 'custom-formats', label: 'Custom formats', href: '#/settings/custom-formats' },
];

/** How to change config for each source (one config source). */
export function changeHow(kind: string | undefined, path?: string): string {
  switch (kind) {
    case 'file': return `Edit ${path ?? 'the config file'}; corsarr picks up the change on its own.`;
    case 'args': return 'Change the --section.key=value arguments corsarr starts with, then restart it.';
    case 'env': return 'Change the CORSARR_* environment variables, then restart corsarr.';
    default: return 'Write a config file (download the export below as a start), point --configFile at it, then restart corsarr.';
  }
}

export function sourceName(kind: string | undefined, path?: string): string {
  switch (kind) {
    case 'file': return path ? `the file ${path}` : 'the config file';
    case 'args': return 'command-line arguments';
    case 'env': return 'environment variables';
    default: return 'built-in defaults';
  }
}
