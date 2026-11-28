// Plugin settings schema and the pure logic SchemaForm
// runs on it: defaults, conditional visibility, client-side validation, dirty tracking and the
// value shapes each field molecule wants. The server runs the same validator; its messages win.
import type { MapEntry } from '../components/molecules/MapField/MapField.svelte';
import type { DurationUnit, SizeUnit } from '../format/units';
import { urlProblem } from '../components/molecules/UrlField/UrlField.svelte';
import { testRegex } from '../components/molecules/RegexField/RegexField.svelte';
import { duplicateKeys } from '../components/molecules/MapField/MapField.svelte';

export type SchemaFieldType =
  | 'text' | 'password' | 'number' | 'bool' | 'select' | 'multiselect' | 'duration' | 'size'
  | 'url' | 'path' | 'regex' | 'list' | 'map';

export interface SchemaOption { value: string; label: string; description?: string }

export interface SchemaCondition {
  /** Key of the field this one depends on. */
  field: string;
  /** Shown when the other field equals this value (or one of these values). */
  equals?: string | number | boolean | (string | number | boolean)[];
  /** Shown when the other field is set (non-empty, true, non-zero). */
  set?: boolean;
}

export interface SchemaValidation {
  min?: number;
  max?: number;
  pattern?: string;
  /** Message shown when `pattern` does not match. */
  patternMessage?: string;
  /** URL schemes allowed for `url` fields. */
  schemes?: string[];
  maxItems?: number;
}

export interface SchemaField {
  /** camelCase key under `<section>.<key>.` */
  key: string;
  type: SchemaFieldType;
  label: string;
  help?: string;
  /** Id of the group this field belongs to. */
  group?: string;
  default?: unknown;
  required?: boolean;
  /** Write-only: the form never receives the stored value. */
  secret?: boolean;
  options?: SchemaOption[];
  /** Options are read from the plugin when the form opens. */
  dynamicOptions?: boolean;
  validation?: SchemaValidation;
  visibleWhen?: SchemaCondition;
  placeholder?: string;
  mono?: boolean;
  /** number: unit shown after the input. */
  unit?: string;
  /** list: the name of one entry ("tracker"). */
  itemLabel?: string;
  durationUnits?: DurationUnit[];
  sizeUnits?: SizeUnit[];
  /** select: radios instead of a dropdown (few, described options). */
  appearance?: 'select' | 'radio';
}

export interface SchemaGroup { id: string; label: string; help?: string }

export type SetupFlowKind = 'pinPairing' | 'cookiePaste' | 'loginForm' | 'oauthDevice';

export interface SetupFlowSpec {
  kind: SetupFlowKind;
  /** Button and heading text: "Sign in with Plex". */
  label: string;
  help?: string;
  /** Secret fields the flow fills when it succeeds. */
  fills: string[];
  /** loginForm: ask for a one-time code as well. */
  otp?: boolean;
}

export interface SettingsSchema {
  fields: SchemaField[];
  groups?: SchemaGroup[];
  setup?: SetupFlowSpec[];
}

/** What the server says about one field beyond its value. */
export interface FieldMeta {
  /** Managed by config: `true` or the source key. */
  managed?: boolean | string;
  drift?: string;
  /** Secret: a value is stored. */
  isSet?: boolean;
  /** Secret: the stored value is this reference. */
  ref?: string;
}

export type FormValues = Record<string, unknown>;

/** The empty value of a type, in the shape the field molecule binds. */
export function emptyValue(type: SchemaFieldType): unknown {
  switch (type) {
    case 'bool': return false;
    case 'number': case 'duration': case 'size': return null;
    case 'multiselect': case 'list': case 'map': return [];
    default: return '';
  }
}

const copyList = (v: unknown) => [...(v as string[])];

/** Map form value: entry lists are copied, objects become entries with string values. */
function mapEntries(v: unknown): MapEntry[] {
  if (Array.isArray(v)) return v.map((e: MapEntry) => ({ ...e }));
  return Object.entries(v as Record<string, unknown>).map(([key, value]) => ({ key, value: String(value) }));
}

/** How a present server value is reshaped per type; other types pass through. */
const formShapes = new Map<SchemaFieldType, (v: unknown) => unknown>([
  ['map', mapEntries], ['list', copyList], ['multiselect', copyList], ['select', String],
]);

/** Server value → form value: maps become ordered entries, missing values take the default. */
export function toFormValue(field: SchemaField, raw: unknown): unknown {
  if (field.secret) return '';
  const v = raw === undefined ? field.default : raw;
  if (v === undefined || v === null) return emptyValue(field.type);
  const shape = formShapes.get(field.type);
  return shape ? shape(v) : v;
}

/** Map entries → the saved object: keys trimmed, entries with an empty key dropped. */
export function entriesObject(entries: MapEntry[]): Record<string, string> {
  const out: Record<string, string> = {};
  for (const e of entries) if (e.key.trim()) out[e.key.trim()] = e.value;
  return out;
}

/** Form value → value sent on save: map entries become an object, empty keys dropped. */
export function fromFormValue(field: SchemaField, value: unknown): unknown {
  if (field.type === 'map') return entriesObject(value as MapEntry[]);
  if (field.type === 'list') return (value as string[]).filter((s) => s.trim() !== '');
  return value;
}

export function initialValues(schema: SettingsSchema, values: FormValues = {}): FormValues {
  return Object.fromEntries(schema.fields.map((f) => [f.key, toFormValue(f, values[f.key])]));
}

function isSetValue(v: unknown): boolean {
  if (Array.isArray(v)) return v.length > 0;
  return v !== undefined && v !== null && v !== '' && v !== false && v !== 0;
}

export function isVisible(field: SchemaField, values: FormValues, schema: SettingsSchema): boolean {
  const c = field.visibleWhen;
  if (!c) return true;
  const parent = schema.fields.find((f) => f.key === c.field);
  if (parent && !isVisible(parent, values, schema)) return false;
  const v = values[c.field];
  if (c.equals !== undefined) {
    const want = Array.isArray(c.equals) ? c.equals : [c.equals];
    return want.some((w) => w === v || String(w) === String(v));
  }
  if (c.set !== undefined) return isSetValue(v) === c.set;
  return true;
}

type Check = (value: unknown, r: SchemaValidation, field: SchemaField) => string | undefined;

function rangeProblem(value: unknown, r: SchemaValidation): string | undefined {
  const n = value as number;
  if (r.min !== undefined && n < r.min) return `Use ${r.min} or more`;
  if (r.max !== undefined && n > r.max) return `Use ${r.max} or less`;
  return undefined;
}

function regexProblem(value: unknown): string | undefined {
  const t = testRegex(value as string, '', false);
  return t.kind === 'invalid' ? `This pattern does not compile: ${t.message}` : undefined;
}

const mapProblem: Check = (value) => (duplicateKeys(value as MapEntry[]).size ? 'Each key can appear only once' : undefined);

const itemsProblem: Check = (value, r) =>
  r.maxItems !== undefined && (value as string[]).length > r.maxItems ? `Use at most ${r.maxItems}` : undefined;

/** Text-like fields: the optional pattern. */
function patternProblem(value: unknown, r: SchemaValidation, field: SchemaField): string | undefined {
  if (!r.pattern || typeof value !== 'string' || new RegExp(r.pattern).test(value)) return undefined;
  return r.patternMessage ?? `${field.label} is not in the expected form`;
}

/** The check each type runs on a non-empty value; other types run the pattern check. */
const typeChecks = new Map<SchemaFieldType, Check>([
  ['number', rangeProblem], ['duration', rangeProblem], ['size', rangeProblem],
  ['url', (value, r) => urlProblem(value as string, r.schemes)],
  ['regex', regexProblem],
  ['map', mapProblem],
  ['list', itemsProblem], ['multiselect', itemsProblem],
]);

/** Nothing entered: a number may be 0 and a checkbox is never empty. */
function isEmpty(field: SchemaField, value: unknown): boolean {
  return !isSetValue(value) && !(field.type === 'number' && value === 0) && field.type !== 'bool';
}

/** The message for an empty required field; a stored secret counts as filled. */
function requiredProblem(field: SchemaField, meta: FieldMeta): string | undefined {
  if (field.secret && (meta.isSet || meta.ref)) return undefined;
  return field.type === 'select' ? `Choose a ${field.label.toLowerCase()}` : `${field.label} is required`;
}

/** Client-side check of one visible field; undefined when fine. */
export function validateField(field: SchemaField, value: unknown, meta: FieldMeta = {}): string | undefined {
  if (meta.managed) return undefined;
  const empty = isEmpty(field, value);
  if (field.required && empty) return requiredProblem(field, meta);
  if (empty) return undefined;
  const check = typeChecks.get(field.type) ?? patternProblem;
  return check(value, field.validation ?? {}, field);
}

export function validate(schema: SettingsSchema, values: FormValues, meta: Record<string, FieldMeta> = {}): Record<string, string> {
  const out: Record<string, string> = {};
  for (const f of schema.fields) {
    if (!isVisible(f, values, schema)) continue;
    const msg = validateField(f, values[f.key], meta[f.key]);
    if (msg) out[f.key] = msg;
  }
  return out;
}

const norm = (v: unknown) => JSON.stringify(v);

/** Keys whose form value differs from the baseline. A secret is dirty when a new value is typed. */
export function dirtyKeys(schema: SettingsSchema, baseline: FormValues, values: FormValues, meta: Record<string, FieldMeta> = {}): string[] {
  return schema.fields
    .filter((f) => !meta[f.key]?.managed)
    .filter((f) => (f.secret ? values[f.key] !== '' : norm(values[f.key]) !== norm(baseline[f.key])))
    .map((f) => f.key);
}

/** The changed, visible, unmanaged values in their saved shape. */
export function changes(schema: SettingsSchema, baseline: FormValues, values: FormValues, meta: Record<string, FieldMeta> = {}): FormValues {
  const dirty = new Set(dirtyKeys(schema, baseline, values, meta));
  return Object.fromEntries(
    schema.fields.filter((f) => dirty.has(f.key) && isVisible(f, values, schema)).map((f) => [f.key, fromFormValue(f, values[f.key])]),
  );
}
