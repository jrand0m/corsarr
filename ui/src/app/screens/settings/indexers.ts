// Settings › Indexers: the pure view logic. Plugin manifests become the add and
// edit forms, connection tests become check rows, server refusals become field
// messages. No component state here; the screen and its story share it.
import type { CheckResult } from '../../../lib/components/molecules/CheckResults/CheckResults.svelte';
import type { FieldMeta, FormValues, SettingsSchema } from '../../../lib/forms/schema';
import type { DefinitionEntry, IndexerInput, IndexerRow, IndexerTest, PluginManifest } from '../../api/types';
import { toSchema } from './plugin-form';

export const keyPattern = '^[a-z][a-z0-9]*$';

const modeNames: Record<string, string> = { search: 'text', tv: 'TV', movie: 'movies', music: 'music', book: 'books' };

/** Check rows for one connection test: the connection, then what the indexer can search. */
export function testChecks(t: IndexerTest): CheckResult[] {
  const checks: CheckResult[] = [{
    id: 'connect', name: 'Connection and sign-in', status: t.ok ? 'pass' : 'fail', ms: t.elapsedMs,
    detail: t.ok ? undefined : withClass(t),
  }];
  if (!t.caps) return checks;
  checks.push(modesCheck(t.caps), idsCheck(t.caps), categoriesCheck(t.caps));
  return checks;
}

type Caps = NonNullable<IndexerTest['caps']>;

/** The search modes the indexer declares, by their plain names. */
function modesCheck(caps: Caps): CheckResult {
  const modes = (caps.modes ?? []).map((m) => modeNames[m] ?? m);
  return { id: 'modes', name: 'Search modes', status: modes.length ? 'pass' : 'warn', detail: modes.length ? modes.join(', ') : 'No search modes declared' };
}

/** The ids the indexer searches by; none means text searches only. */
function idsCheck(caps: Caps): CheckResult {
  const ids = caps.idParams ?? [];
  return { id: 'ids', name: 'Searches by id', status: ids.length ? 'pass' : 'skipped', detail: ids.length ? ids.join(', ') : 'Text searches only' };
}

/** How many categories the indexer maps. */
function categoriesCheck(caps: Caps): CheckResult {
  const cats = Object.keys(caps.categories ?? {}).length;
  return { id: 'cats', name: 'Categories', status: cats ? 'pass' : 'warn', detail: cats ? `${cats} mapped` : 'No categories; every search covers all of them' };
}

function withClass(t: IndexerTest): string {
  return t.class ? `${t.message} (${t.class.replace('_', ' ')})` : t.message;
}

export interface PluginChoice { value: string; label: string; description: string }

/** Indexer plugins to add, built-ins first, each described by its delivery and definition facts. */
export function pluginChoices(manifests: PluginManifest[], definitions: DefinitionEntry[]): PluginChoice[] {
  const defs = new Map(definitions.filter((d) => d.valid).map((d) => [d.id, d]));
  return manifests
    .filter((m) => m.kind === 'indexer')
    .map((m) => ({ value: m.id, label: m.name, description: describe(m, defs.get(m.id)) }))
    .sort((a, b) => rank(a.description) - rank(b.description) || a.label.localeCompare(b.label));
}

function describe(m: PluginManifest, d?: DefinitionEntry): string {
  if (m.delivery !== 'declarative') return 'Built in';
  const parts = ['Definition', d?.privacy, d?.language, d?.origin === 'user' ? 'your file' : undefined];
  return parts.filter(Boolean).join(' · ');
}

const rank = (description: string) => (description === 'Built in' ? 0 : 1);

/** A free key from a display name: "The Pirate Bay" → "thepiratebay", then "thepiratebay2". */
export function suggestKey(name: string, taken: string[]): string {
  let base = name.toLowerCase().replace(/[^a-z0-9]/g, '').replace(/^[0-9]+/, '');
  if (!base) base = 'indexer';
  const used = new Set(taken);
  if (!used.has(base)) return base;
  let n = 2;
  while (used.has(`${base}${n}`)) n++;
  return `${base}${n}`;
}

/** The add form: key and name, then the plugin's own settings. */
export function addSchema(m: PluginManifest): SettingsSchema {
  const s = toSchema(m);
  return {
    fields: [
      { key: 'key', type: 'text', label: 'Key', required: true, mono: true, validation: { pattern: keyPattern, patternMessage: 'Lower-case letters and digits, starting with a letter.' },
        help: 'Stable id used in the Torznab address and in config keys. It cannot change later.' },
      ...s.fields,
    ],
  };
}

/** Form values (from SchemaForm's changed values) as a create request. */
export function toInput(plugin: string, v: FormValues): IndexerInput {
  const { key, name, ...settings } = v;
  return { key: String(key ?? ''), plugin, name: name ? String(name) : undefined, settings };
}

/** Changed values of the edit form as an edit request. */
export function toEdit(v: FormValues): IndexerInput {
  const { name, ...settings } = v;
  const out: IndexerInput = {};
  if (name !== undefined) out.name = String(name);
  if (Object.keys(settings).length) out.settings = settings;
  return out;
}

/** Schema, values and meta to edit an indexer; config indexers are locked to their config keys. */
export function editView(row: IndexerRow, m: PluginManifest): { schema: SettingsSchema; values: FormValues; meta: Record<string, FieldMeta> } {
  const schema = toSchema(m);
  const values: FormValues = { name: row.name };
  const meta: Record<string, FieldMeta> = {};
  const managed = (k: string) => (row.source === 'config' ? `indexers.${row.key}.${k}` : undefined);
  if (managed('name')) meta.name = { managed: managed('name') };
  for (const f of m.settings) {
    const lock = managed(f.key);
    if (row.secretsSet.includes(f.key)) meta[f.key] = { isSet: true, managed: lock };
    else if (lock) meta[f.key] = { managed: lock };
    if (row.settings[f.key] !== undefined) values[f.key] = row.settings[f.key] as FormValues[string];
  }
  return { schema, values, meta };
}

/** Field messages from a refused save: `{fields: [{field, message}]}`. */
export function fieldErrors(body: Record<string, unknown> | undefined): Record<string, string> {
  const out: Record<string, string> = {};
  const fields = (body?.fields ?? []) as { field: string; message: string }[];
  for (const f of fields) out[f.field] = `${f.message}.`.replace(/\.\.$/, '.');
  return out;
}

/** The connection test carried by a refused save, when the refusal was a failed test. */
export function refusedTest(body: Record<string, unknown> | undefined): IndexerTest | undefined {
  return body?.test as IndexerTest | undefined;
}

/** One line per definition problem: "search.response.rows (line 12): …". */
export function problemLines(e: DefinitionEntry): string[] {
  return (e.problems ?? []).map((p) => `${p.path ? p.path : 'file'}${p.line ? ` (line ${p.line})` : ''}: ${p.message}`);
}

/** Health status shown for a row: disabled when switched off, unchecked when unknown. */
export function rowStatus(row: IndexerRow): string | undefined {
  if (!row.enabled) return 'disabled';
  return row.health?.status;
}

/** Absolute Torznab address for a path served by this corsarr. */
export function torznabURL(path: string, origin: string): string {
  return `${origin.replace(/\/$/, '')}${path}`;
}
