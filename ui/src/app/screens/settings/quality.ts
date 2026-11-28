// Settings › Profiles and Custom formats: the pure view logic. Profile ladders
// become ordered rows (best on top) and back, conditions get one-line
// summaries, server refusals become field messages per condition or field.
import type { OrderedItem } from '../../../lib/components/organisms/OrderedListEditor/OrderedListEditor.svelte';
import type { MediaKind } from '../../../lib/tokens';
import type {
  ConditionDoc, ConditionKindInfo, FormatDoc, GroupDoc, ProfileDoc, ProfileRow, QualityChoice,
} from '../../api/types';

/** "TV and movies", "Music". */
export function kindsLabel(kinds: MediaKind[]): string {
  const names: Record<string, string> = { tv: 'TV', movie: 'movies', music: 'music', book: 'books' };
  const s = kinds.map((k) => names[k] ?? k).join(' and ');
  return s.charAt(0).toUpperCase() + s.slice(1);
}

const qualityId = (id: string) => `q:${id}`;

/**
 * Ladder rows for the order editor: the allowed ranks best first (a group of
 * several qualities is one row with children), then every other quality of
 * the kind switched off, best first. Qualities below the floor are locked.
 */
export function ladderItems(groups: GroupDoc[], ladder: QualityChoice[]): OrderedItem[] {
  const names = new Map(ladder.map((q) => [q.id, q.name]));
  const used = new Set(groups.flatMap((g) => g.qualities));
  const rows: OrderedItem[] = [...groups].reverse().map((g, i) => {
    if (g.qualities.length === 1 && !g.name) {
      return { id: qualityId(g.qualities[0]), label: names.get(g.qualities[0]) || g.qualities[0], enabled: true };
    }
    return {
      id: `g:${i}:${g.name ?? ''}`, label: g.name || g.qualities.map((q) => names.get(q) || q).join(' + '), enabled: true,
      children: g.qualities.map((q) => ({ id: qualityId(q), label: names.get(q) || q, enabled: true })),
    };
  });
  const rest = [...ladder].reverse().filter((q) => !used.has(q.id)).map((q): OrderedItem => ({
    id: qualityId(q.id), label: q.name, enabled: false,
    locked: q.belowFloor ? 'Below the quality floor: never grabbed' : undefined,
  }));
  return [...rows, ...rest];
}

const idOf = (item: OrderedItem) => item.id.replace(/^q:/, '');

/** The allowed ranks of the order editor's rows, worst first. */
export function groupsFromItems(items: OrderedItem[]): GroupDoc[] {
  const out: GroupDoc[] = [];
  for (const it of [...items].reverse()) {
    if (it.enabled === false) continue;
    if (it.children?.length) {
      const qualities = it.children.filter((c) => c.enabled !== false).map(idOf);
      if (qualities.length) out.push({ name: it.label, qualities });
    } else {
      out.push({ qualities: [idOf(it)] });
    }
  }
  return out;
}

/** Cutoff choices: every allowed quality, best first, named with its group. */
export function cutoffOptions(groups: GroupDoc[], ladder: QualityChoice[]): { value: string; label: string }[] {
  const names = new Map(ladder.map((q) => [q.id, q.name]));
  return [...groups].reverse().flatMap((g) => g.qualities.map((q) => ({
    value: q, label: g.name && g.qualities.length > 1 ? `${names.get(q) || q} (${g.name})` : names.get(q) || q,
  })));
}

/** Keeps the cutoff on an allowed quality after the ladder changed: else the best allowed one. */
export function repairCutoff(cutoff: string, groups: GroupDoc[]): string {
  const allowed = groups.flatMap((g) => g.qualities);
  return allowed.includes(cutoff) ? cutoff : allowed[allowed.length - 1] ?? '';
}

/** "1–10 GB", "above 5 GB". */
function rangeText(c: ConditionDoc, unit: string | undefined): string {
  const u = unit ? ` ${unit}` : '';
  return c.max ? `${c.min ?? 0}–${c.max}${u}` : `above ${c.min ?? 0}${u}`;
}

/** The chosen values by their offered names; values the kind does not offer stay as they are. */
const valueNames = (c: ConditionDoc, info: ConditionKindInfo | undefined): string[] =>
  (c.values ?? []).map((v) => info?.values?.find((o) => o.value === v)?.label || v);

/** "AAC", "AAC or AC3", "AAC, DTS or AC3". */
const orList = (names: string[]): string =>
  names.length > 1 ? `${names.slice(0, -1).join(', ')} or ${names[names.length - 1]}` : names[0] ?? '';

/** "Audio codec is AAC or AC3", "Audio codec is anything but AAC". */
function valuesSummary(label: string, not: string, c: ConditionDoc, info: ConditionKindInfo | undefined): string {
  const list = orList(valueNames(c, info));
  return c.except ? `${label} is anything but ${list}` : `${label} is${not} ${list}`;
}

/** "Audio codec is AAC or AC3", "Release title does not match \bDTS\b", "Size 1–10 GB". */
export function conditionSummary(c: ConditionDoc, kinds: ConditionKindInfo[]): string {
  const info = kinds.find((k) => k.kind === c.kind);
  const label = info?.label || c.kind;
  const not = c.negate ? ' not' : '';
  switch (info?.input) {
    case 'pattern': return `${label} does${not} match ${c.pattern ?? ''}`;
    case 'range': return `${label} is${not} ${rangeText(c, info.unit)}`;
    default: return valuesSummary(label, not, c, info);
  }
}

/** Summary of a whole format for the list: first two conditions and a count. */
export function formatSummary(f: FormatDoc, kinds: ConditionKindInfo[]): string {
  const parts = f.conditions.slice(0, 2).map((c) => conditionSummary(c, kinds));
  const more = f.conditions.length - parts.length;
  return more > 0 ? `${parts.join('; ')} (+${more} more)` : parts.join('; ');
}

export interface Refused { fields: Record<string, string>; conditions: Record<string, string>[]; message?: string }

/**
 * Splits a 422 body into field messages: "conditions.2.pattern" goes to the
 * third condition's "pattern"; "groups.0.qualities.1" and the like stay
 * whole under their first segment.
 */
export function refusal(body: Record<string, unknown> | undefined, message?: string): Refused {
  const out: Refused = { fields: {}, conditions: [], message };
  const fields = (body?.fields ?? []) as { field: string; message: string }[];
  for (const f of fields) routeRefusal(out, f);
  return out;
}

/** Puts one refused field's message on its condition field, or on its top-level field (the form when unnamed). */
function routeRefusal(out: Refused, f: { field: string; message: string }): void {
  const m = /^conditions\.(\d+)\.(\w+)/.exec(f.field ?? '');
  if (m) {
    const i = Number(m[1]);
    out.conditions[i] = { ...(out.conditions[i] ?? {}), [m[2]]: f.message };
    return;
  }
  const top = (f.field ?? '').split('.')[0] || 'form';
  out.fields[top] = out.fields[top] ? `${out.fields[top]} ${f.message}` : f.message;
}

export function emptyFormat(): FormatDoc {
  return { key: '', name: '', conditions: [{ kind: 'title', pattern: '' }] };
}

/** The document a row edits (rows carry read-only facts the API refuses). */
export function profileDoc(r: ProfileRow): ProfileDoc {
  const { builtin: _b, origin: _o, managed: _m, items: _i, default: _d, ...doc } = r as ProfileRow & { isDefault?: boolean };
  delete (doc as { isDefault?: boolean }).isDefault;
  return doc;
}

export function formatDoc(r: FormatDoc & { origin?: unknown; managed?: unknown; usedBy?: unknown }): FormatDoc {
  return { key: r.key, name: r.name, conditions: r.conditions };
}

/** Where a row comes from, in plain words. */
export function originLabel(r: { origin: string }): string {
  return r.origin === 'builtin' ? 'Built-in' : r.origin === 'config' ? 'Config file' : 'Added here';
}
