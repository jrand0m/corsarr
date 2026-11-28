// A module's settings schema (a closed set of field types the server already checked) → the
// SchemaForm schema, plus the value conversions between the two, the permission diff an update
// shows and the YAML snippet for a new instance. Pure. Every text from the module stays a string:
// it becomes a label, help or option label prop and is never parsed or turned into markup.
import type { FieldMeta, FormValues, SchemaField, SettingsSchema } from './schema';
import type { MapEntry } from '../components/molecules/MapField/MapField.svelte';
import type { ModuleAction, ModuleField, ModulePermissions, ModuleSchema, PermissionDiff, Problem } from '../../app/api/types';

/** The schema holds something the closed set does not: the form shows nothing from it. */
export class UnsupportedSchema extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'UnsupportedSchema';
  }
}

export interface PluginForm {
  schema: SettingsSchema;
  actions: ModuleAction[];
  /** Plain-text hints by field key, added to the field's validation message. */
  hints: Record<string, string>;
}

const MAX_FIELDS = 50;
const MAX_GROUPS = 10;
const MAX_ACTIONS = 3;
const IDENT = '^[a-z][a-zA-Z0-9]{0,47}$';
const identRe = new RegExp(IDENT);
const HOSTNAME = '^(?=.{1,253}$)[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$';

const types = new Set(['string', 'secret', 'bool', 'int', 'number', 'enum', 'duration', 'list<string>', 'list<int>', 'map<string,string>']);
const patterns = new Set(['hostname', 'url-http', 'path-absolute', 'identifier']);

function stringField(f: ModuleField, base: SchemaField): SchemaField {
  switch (f.pattern) {
    case undefined: case '': return base;
    case 'url-http': return { ...base, type: 'url', mono: true, validation: { schemes: ['http', 'https'] } };
    case 'path-absolute': return { ...base, type: 'path', mono: true };
    case 'hostname': return { ...base, mono: true, validation: { pattern: HOSTNAME, patternMessage: `${f.label} must be a host name` } };
    case 'identifier': return { ...base, mono: true, validation: { pattern: IDENT, patternMessage: `${f.label} must start with a lower-case letter followed by letters or digits` } };
    default: throw new UnsupportedSchema(`field ${f.key}: unknown pattern`);
  }
}

const bounds = (min?: number, max?: number) => (min !== undefined || max !== undefined ? { validation: { min, max } } : {});

function toField(f: ModuleField): SchemaField {
  if (!f || typeof f !== 'object') throw new UnsupportedSchema('a field is not an object');
  if (typeof f.key !== 'string' || !identRe.test(f.key)) throw new UnsupportedSchema('a field key is not an identifier');
  if (!types.has(f.type)) throw new UnsupportedSchema(`field ${f.key}: unsupported type`);
  if (f.pattern && !patterns.has(f.pattern)) throw new UnsupportedSchema(`field ${f.key}: unknown pattern`);
  const base: SchemaField = { key: f.key, type: 'text', label: String(f.label ?? f.key), help: f.description, required: f.required || undefined };
  if (f.default !== undefined) base.default = f.default;
  switch (f.type) {
    case 'string': return stringField(f, base);
    case 'secret': return { ...base, secret: true, default: undefined };
    case 'bool': return { ...base, type: 'bool' };
    case 'int': case 'number': return { ...base, type: 'number', ...bounds(f.min, f.max) };
    case 'enum': return { ...base, type: 'select', options: (f.options ?? []).map((o) => ({ value: String(o.value), label: String(o.label) })) };
    case 'duration': {
      const lo = f.minDuration ? parseGoDuration(f.minDuration) ?? undefined : undefined;
      const hi = f.maxDuration ? parseGoDuration(f.maxDuration) ?? undefined : undefined;
      const def = typeof f.default === 'string' ? parseGoDuration(f.default) ?? undefined : undefined;
      return { ...base, type: 'duration', default: def, ...bounds(lo, hi) };
    }
    case 'list<string>': return { ...base, type: 'list', validation: f.maxItems ? { maxItems: f.maxItems } : undefined };
    case 'list<int>': return { ...base, type: 'list', mono: true, default: Array.isArray(f.default) ? f.default.map(String) : undefined, validation: f.maxItems ? { maxItems: f.maxItems } : undefined };
    default: return { ...base, type: 'map', mono: true };
  }
}

/** The SchemaForm schema, the actions and the hints of a module schema; throws UnsupportedSchema. */
export function toFormSchema(s: ModuleSchema): PluginForm {
  if (!s || !Array.isArray(s.fields)) throw new UnsupportedSchema('the schema has no field list');
  if (s.fields.length > MAX_FIELDS) throw new UnsupportedSchema(`more than ${MAX_FIELDS} fields`);
  const groups = s.groups ?? [];
  const actions = s.actions ?? [];
  if (groups.length > MAX_GROUPS) throw new UnsupportedSchema(`more than ${MAX_GROUPS} groups`);
  if (actions.length > MAX_ACTIONS) throw new UnsupportedSchema(`more than ${MAX_ACTIONS} actions`);
  for (const a of actions) if (!identRe.test(String(a?.id))) throw new UnsupportedSchema('an action id is not an identifier');

  const fields = s.fields.map(toField);
  const byKey = new Map(fields.map((f) => [f.key, f]));
  if (byKey.size !== fields.length) throw new UnsupportedSchema('two fields share a key');
  const grouped: SchemaField[] = [];
  groups.forEach((g, i) => {
    for (const key of g.fields ?? []) {
      const f = byKey.get(key);
      if (!f || f.group) throw new UnsupportedSchema(`group ${i + 1} lists an unknown or repeated field`);
      f.group = `g${i}`;
      grouped.push(f);
    }
  });
  const hints = Object.fromEntries(s.fields.filter((f) => f.hint).map((f) => [f.key, String(f.hint)]));
  return {
    schema: {
      fields: [...fields.filter((f) => !f.group), ...grouped],
      groups: groups.length ? groups.map((g, i) => ({ id: `g${i}`, label: String(g.title), help: g.description })) : undefined,
    },
    actions: actions.map((a) => ({ id: a.id, label: String(a.label) })),
    hints,
  };
}

// ── durations ───────────────────────────────────────────────────────────────

const durUnits: Record<string, number> = { h: 3600, m: 60, s: 1, ms: 0.001 };

/** "1h2m3s" → 3723 seconds; null when it is not a Go duration. */
export function parseGoDuration(s: string): number | null {
  const t = s.trim();
  if (!/^(\d+(\.\d+)?(h|ms|m|s))+$/.test(t)) return null;
  let total = 0;
  for (const m of t.matchAll(/(\d+(?:\.\d+)?)(h|ms|m|s)/g)) total += Number(m[1]) * durUnits[m[2]];
  return Math.round(total * 1000) / 1000;
}

/** 3720 seconds → "1h2m". */
export function formatGoDuration(sec: number): string {
  if (!sec) return '0s';
  const h = Math.floor(sec / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = Math.round((sec % 60) * 1000) / 1000;
  return `${h ? `${h}h` : ''}${m ? `${m}m` : ''}${s ? `${s}s` : ''}`;
}

// ── values ──────────────────────────────────────────────────────────────────

/** Config or server values (the env export gives strings) → the shapes SchemaForm binds. */
export function toFormValues(fields: ModuleField[], raw: Record<string, unknown>): FormValues {
  const out: FormValues = {};
  for (const f of fields) {
    const v = raw[f.key];
    if (v === undefined || v === null || f.type === 'secret') continue;
    switch (f.type) {
      case 'int': case 'number': out[f.key] = Number(v); break;
      case 'bool': out[f.key] = v === true || v === 'true'; break;
      case 'duration': out[f.key] = typeof v === 'number' ? v : parseGoDuration(String(v)); break;
      case 'list<int>': case 'list<string>': out[f.key] = (Array.isArray(v) ? v : [v]).filter((x) => x !== undefined).map(String); break;
      default: out[f.key] = v;
    }
  }
  return out;
}

/** SchemaForm values → the module types; empty values and untyped secrets are left out. */
export function fromFormValues(fields: ModuleField[], values: FormValues): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const f of fields) {
    const v = values[f.key];
    if (v === undefined || v === null || v === '') continue;
    switch (f.type) {
      case 'duration': out[f.key] = formatGoDuration(Number(v)); break;
      case 'list<int>': out[f.key] = (v as string[]).map((s) => String(s).trim()).filter(Boolean).map(Number); break;
      case 'list<string>': out[f.key] = (v as string[]).filter((s) => String(s).trim() !== ''); break;
      case 'map<string,string>': {
        if (!Array.isArray(v)) { out[f.key] = v; break; }
        const m: Record<string, string> = {};
        for (const e of v as MapEntry[]) if (e.key.trim()) m[e.key.trim()] = e.value;
        out[f.key] = m;
        break;
      }
      default: out[f.key] = v;
    }
  }
  return out;
}

/** Managed meta for a configured instance: every field locked with its key, secrets by reference. */
export function instanceMeta(fields: ModuleField[], section: string, key: string, secretRefs: Record<string, string> = {}): Record<string, FieldMeta> {
  const meta: Record<string, FieldMeta> = {};
  for (const f of fields) {
    const managed = `${section}.${key}.${f.key}`;
    const ref = secretRefs[f.key.toLowerCase()];
    meta[f.key] = f.type === 'secret' && ref ? { managed, isSet: true, ref: `\${env:${ref}}` } : { managed };
  }
  return meta;
}

/** Server problems → messages by field key (with the plugin's hint) and the rest as lines. */
export function problemErrors(problems: Problem[], keys: string[], hints: Record<string, string> = {}): { fields: Record<string, string>; other: string[] } {
  const fields: Record<string, string> = {};
  const other: string[] = [];
  for (const p of problems) {
    const msg = String(p.message ?? p.msg ?? '');
    const key = p.path.replace(/^(values|settings)\./, '').split(/[.[]/)[0];
    if (keys.includes(key)) {
      if (!fields[key]) fields[key] = hints[key] ? `${msg}. ${hints[key]}` : msg;
    } else other.push(p.path ? `${p.path}: ${msg}` : msg);
  }
  return { fields, other };
}

// ── permissions ─────────────────────────────────────────────────────────────

/** What an update adds and drops compared with the approved permissions. */
export function permissionDiff(old: ModulePermissions | undefined, next: ModulePermissions): PermissionDiff {
  const oh = old?.http?.hosts ?? [];
  const nh = next.http?.hosts ?? [];
  const os = old?.secrets ?? [];
  const ns = next.secrets ?? [];
  return {
    addedHosts: nh.filter((h) => !oh.includes(h)),
    removedHosts: oh.filter((h) => !nh.includes(h)),
    addedSecrets: ns.filter((s) => !os.includes(s)),
    removedSecrets: os.filter((s) => !ns.includes(s)),
  };
}

// ── config snippet ──────────────────────────────────────────────────────────

const sections: Record<string, string> = {
  downloader: 'downloadClients', indexer: 'indexers', notifier: 'notifications', mediaserver: 'mediaServers', metadata: 'metadata', source: 'sources', chatbot: 'chatbots',
};
/** The config section that holds instances of a module kind. */
export const sectionFor = (kind: string): string => sections[kind] ?? kind;

/** The env variable a secret reference names: CORSARR_SECRET_<SECTION>_<KEY>_<FIELD>. */
export const secretVar = (section: string, key: string, field: string) => `CORSARR_SECRET_${section}_${key}_${field}`.toUpperCase();

const plain = /^[A-Za-z0-9_./@-][A-Za-z0-9_ ./:@-]*$/;
const reserved = /^(true|false|yes|no|on|off|y|n|null|~|[-+]?(\d[\d_]*)?(\.\d+)?([eE][-+]?\d+)?|0x[0-9a-fA-F]+|\.inf|\.nan)$/i;

/** A YAML scalar: plain when it reads back as the same string, else double-quoted. */
function scalar(v: unknown): string {
  if (typeof v === 'number' || typeof v === 'boolean') return String(v);
  const s = String(v);
  if (s && plain.test(s) && !reserved.test(s) && !/\s$/.test(s) && !/: /.test(s)) return s;
  return JSON.stringify(s);
}
const isEnvRef = (s: unknown) => typeof s === 'string' && /^\$\{env:[A-Z0-9_]+\}$/.test(s);

export interface SnippetInput {
  section: string;
  key: string;
  plugin: string;
  name?: string;
  fields: ModuleField[];
  /** Values in the module types (fromFormValues). */
  values: Record<string, unknown>;
}

/**
 * YAML for a new instance under its config section. Secret fields that were typed (or are
 * required) become `${env:CORSARR_SECRET_…}` references; the typed value never appears.
 * Values equal to the field's default are left out.
 */
export function configSnippet(i: SnippetInput): string {
  const lines = [`${i.section}:`, `  ${i.key}:`, `    type: ${scalar(i.plugin)}`];
  if (i.name) lines.push(`    name: ${scalar(i.name)}`);
  for (const f of i.fields) {
    const v = i.values[f.key];
    if (f.type === 'secret') {
      if ((v !== undefined && v !== '') || f.required) lines.push(`    ${f.key}: \${env:${secretVar(i.section, i.key, f.key)}}`);
      continue;
    }
    if (v === undefined || v === null || v === '') continue;
    if (f.default !== undefined && JSON.stringify(f.default) === JSON.stringify(v)) continue;
    if (Array.isArray(v)) {
      if (!v.length) continue;
      lines.push(`    ${f.key}:`, ...v.map((x) => `      - ${scalar(x)}`));
    } else if (typeof v === 'object') {
      const entries = Object.entries(v as Record<string, unknown>);
      if (!entries.length) continue;
      lines.push(`    ${f.key}:`, ...entries.map(([k, x]) => `      ${scalar(k)}: ${scalar(x)}`));
    } else lines.push(`    ${f.key}: ${isEnvRef(v) ? v : scalar(v)}`);
  }
  return `${lines.join('\n')}\n`;
}
