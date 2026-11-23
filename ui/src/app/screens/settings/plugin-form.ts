// Plugin manifest settings → the SchemaForm schema, with the configured
// instance's values and a managed lock on every field. Pure.
import type { FieldMeta, FormValues, SchemaField, SchemaFieldType, SettingsSchema } from '../../../lib/forms/schema';
import type { PluginField, PluginManifest } from '../../api/types';
import type { ConfiguredInstance } from '../../model';

const types: Record<PluginField['type'], SchemaFieldType> = {
  text: 'text', secret: 'text', number: 'number', bool: 'bool', enum: 'select', multiselect: 'multiselect', duration: 'duration',
  size: 'size', url: 'url', path: 'path', regex: 'regex', list: 'list', map: 'map',
};

export function toSchemaField(f: PluginField): SchemaField {
  const field: SchemaField = { key: f.key, type: types[f.type] ?? 'text', label: f.label, help: f.help, required: f.required, default: f.default };
  if (f.type === 'secret' || f.secret) field.secret = true;
  if (f.options) field.options = f.options.map((o) => ({ value: o, label: o }));
  if (f.min !== undefined || f.max !== undefined) field.validation = { min: f.min, max: f.max };
  if (f.showIf) {
    const [k, v] = f.showIf.split('=');
    field.visibleWhen = { field: k, equals: v === 'true' ? true : v === 'false' ? false : v };
  }
  if (f.type === 'url' || f.type === 'path' || f.type === 'regex') field.mono = true;
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
