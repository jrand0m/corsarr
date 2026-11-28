// Pure helpers of the plugin module screens: the config lock on module changes, refusal wording,
// permission summaries and the configured values of the instances that use a module.
import { managedFrom, type ManagedSource } from '../../../lib/components/atoms/ManagedValue/ManagedValue.svelte';
import { instanceMeta } from '../../../lib/forms/plugin-schema';
import type { FieldMeta } from '../../../lib/forms/schema';
import type { InstanceRef, ModuleInfo, ModulePermissions, PluginPolicy, Problem } from '../../api/types';
import type { ConfiguredInstance } from '../../model';

/** Where the config turns module changes off, or undefined when the web UI may change modules. */
export function lockOf(policy: PluginPolicy | undefined, source?: { kind?: string; path?: string }): ManagedSource | undefined {
  if (!policy || policy.uiInstall) return undefined;
  return managedFrom(policy.lockKey || 'plugins.uiInstall', source);
}

/** The words for each refused module change, by error code; other codes show the server's message. */
const changeErrors = new Map<string, (message: string) => string>([
  ['plugins_locked', (m) => `Module changes are off in the config. ${m}`],
  ['catalogue_changed', () => 'The catalogue now offers another version. Check the new entry and confirm again.'],
  ['verify_failed', (m) => `The file did not pass the checks: ${m}`],
  ['incompatible', (m) => `This version does not run on this corsarr: ${m}`],
  ['no_previous', () => 'There is no previous version to go back to.'],
  ['modules_unavailable', () => 'Plugin modules are switched off on this server.'],
  ['catalogue_unreachable', (m) => `The catalogue is not reachable: ${m}`],
]);

/** The words for a refused module change, by error code. */
export function changeError(code: string, message: string): string {
  const words = changeErrors.get(code);
  return words ? words(message) : message;
}

/** "1 host", "2 secrets". */
const counted = (n: number, one: string) => `${n} ${n === 1 ? one : `${one}s`}`;

/** How many hosts and secrets a module may reach, with the word for one of each. */
const accessCounts = (p: ModulePermissions | undefined): [number, string][] => [
  [p?.http?.hosts?.length ?? 0, 'host'],
  [p?.secrets?.length ?? 0, 'secret'],
];

/** "2 hosts · 1 secret", "No access". */
export function permissionSummary(p: ModulePermissions | undefined): string {
  const parts = accessCounts(p).filter(([n]) => n > 0).map(([n, one]) => counted(n, one));
  return parts.length ? parts.join(' · ') : 'No access';
}

/** Field labels by key, for permission wording. */
export const fieldLabels = (m: ModuleInfo): Record<string, string> =>
  Object.fromEntries((m.manifest.settings?.fields ?? []).map((f) => [f.key, f.label]));

export interface InstanceView {
  ref: InstanceRef;
  values: Record<string, unknown>;
  meta: Record<string, FieldMeta>;
}

/** The configured values (from the env export) and managed locks of each instance using a module. */
export function instanceViews(m: ModuleInfo, configured: ConfiguredInstance[]): InstanceView[] {
  const fields = m.manifest.settings?.fields ?? [];
  return (m.usedBy ?? []).map((ref) => {
    const c = configured.find((i) => i.section === ref.section && i.key === ref.key.toLowerCase());
    const values: Record<string, unknown> = {};
    for (const f of fields) {
      const v = c?.values[f.key.toLowerCase()];
      if (v !== undefined) values[f.key] = v;
    }
    return { ref, values, meta: instanceMeta(fields, ref.section, ref.key, c?.secrets) };
  });
}

/** Problems from a 422 body (`problems`), or none. */
export function problemsOf(body: Record<string, unknown> | undefined): Problem[] {
  const p = body?.problems;
  return Array.isArray(p) ? (p as Problem[]) : [];
}

export const sortModules = (rows: ModuleInfo[]) => [...rows].sort((a, b) => (a.manifest.name || a.id).localeCompare(b.manifest.name || b.id));
