// Plugin manifest settings → the SchemaForm schema, with the configured
// instance's values and a managed lock on every field. Pure.
import type { FieldMeta, FormValues, SchemaCondition, SchemaField, SchemaFieldType, SettingsSchema } from '../../../lib/forms/schema';
import type { PluginField, PluginManifest } from '../../api/types';
import type { ConfiguredInstance } from '../../model';

const types: Record<PluginField['type'], SchemaFieldType> = {
  text: 'text', secret: 'text', number: 'number', bool: 'bool', enum: 'select', multiselect: 'multiselect', duration: 'duration',
  size: 'size', url: 'url', path: 'path', regex: 'regex', list: 'list', map: 'map',
};

const monoTypes = new Set<PluginField['type']>(['url', 'path', 'regex']);

/** "key=value" → shown when `key` equals the value, read as a boolean when it is true or false. */
function showIfCondition(showIf: string): SchemaCondition {
  const [k, v] = showIf.split('=');
  return { field: k, equals: v === 'true' ? true : v === 'false' ? false : v };
}

/** Adds the options, bounds and condition the manifest field declares. */
function addRules(field: SchemaField, f: PluginField): void {
  if (f.options) field.options = f.options.map((o) => ({ value: o, label: o }));
  if (f.min !== undefined || f.max !== undefined) field.validation = { min: f.min, max: f.max };
  if (f.showIf) field.visibleWhen = showIfCondition(f.showIf);
}

export function toSchemaField(f: PluginField): SchemaField {
  const field: SchemaField = { key: f.key, type: types[f.type] ?? 'text', label: f.label, help: f.help, required: f.required, default: f.default };
  if (f.type === 'secret' || f.secret) field.secret = true;
  addRules(field, f);
  if (monoTypes.has(f.type)) field.mono = true;
  return field;
}

export function toSchema(m: PluginManifest): SettingsSchema {
  return { fields: [{ key: 'name', type: 'text', label: 'Name' }, ...m.settings.map(toSchemaField)] };
}

/** Schema, values and managed meta for one configured instance. */
export function instanceView(i: ConfiguredInstance, m: PluginManifest): { schema: SettingsSchema; values: FormValues; meta: Record<string, FieldMeta> } {
  const schema = toSchema(m);
  const values: FormValues = { name: i.name ?? i.key };
  const meta: Record<string, FieldMeta> = { name: { managed: `${i.section}.${i.key}.name` } };
  for (const f of m.settings) {
    const low = f.key.toLowerCase();
    const managed = `${i.section}.${i.key}.${f.key}`;
    if (i.secrets[low]) meta[f.key] = { managed, isSet: true, ref: `\${env:${i.secrets[low]}}` };
    else meta[f.key] = { managed };
    const v = i.values[low];
    if (v === undefined) continue;
    if (f.type === 'number') values[f.key] = Number(v);
    else if (f.type === 'bool') values[f.key] = v === 'true';
    else values[f.key] = v;
  }
  return { schema, values, meta };
}

const sectionLabels: Record<string, string> = {
  indexers: 'Indexers', sources: 'Sources', downloadClients: 'Download clients', mediaServers: 'Media servers',
  notifications: 'Notifications', metadata: 'Metadata', chatbots: 'Chat bots',
};
export const sectionLabel = (s: string) => sectionLabels[s] ?? s;
