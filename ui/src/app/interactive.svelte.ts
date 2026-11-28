// Interactive search state for a drawer: streams NDJSON decisions as indexers answer, keeps
// per-indexer progress, ranks the releases best first and turns each into a DecisionPanel
// decision. A grab goes through the override rules: 409 asks to confirm, 403 refuses.
import type { SourceProgress } from '../lib/components/molecules/SourceProgressStrip/SourceProgressStrip.svelte';
import type { AutomationOutcome, Decision as PanelDecision, DecisionReason } from '../lib/components/organisms/DecisionPanel/DecisionPanel.svelte';
import { currentRoute, replaceParams } from '../lib/route/hash';
import { ApiError, toApiError, type Client } from './api/client';
import type { AutomationVerdict, Decided, ReleaseLine } from './api/types';

type Summary = Extract<ReleaseLine, { summary: unknown }>['summary'];

/** Why a grab did not go through. */
export interface GrabFailure { confirm: boolean; refused: boolean; message: string; reasons: string[] }

/** Ranks decisions: accepted first, then by rank key, best first. */
export function rank(list: Decided[]): Decided[] {
  return list.slice().sort((a, b) =>
    Number(b.decision.accepted) - Number(a.decision.accepted) || (a.decision.rankKey < b.decision.rankKey ? 1 : a.decision.rankKey > b.decision.rankKey ? -1 : 0));
}

/** The orders the result list offers; automation is the default. */
export type SortKey = 'automation' | 'desirability' | 'seeders' | 'size' | 'newest' | 'quality';
export const sortOptions: { value: SortKey; label: string }[] = [
  { value: 'automation', label: 'Automation order' },
  { value: 'desirability', label: 'Closest to profile' },
  { value: 'seeders', label: 'Most seeders' },
  { value: 'size', label: 'Largest first' },
  { value: 'newest', label: 'Newest first' },
  { value: 'quality', label: 'Best quality' },
];
const isSortKey = (v: string | null): v is SortKey => sortOptions.some((o) => o.value === v);

/** Reads the sort from the page URL (`?sort=`), automation when absent or unknown. */
export function readSort(params: URLSearchParams = currentRoute().params): SortKey {
  const v = params.get('sort');
  return isSortKey(v) ? v : 'automation';
}

/** Keeps the sort in the page URL; the default is left out. */
export function writeSort(key: SortKey): void {
  const p = currentRoute().params;
  if (key === 'automation') p.delete('sort'); else p.set('sort', key);
  replaceParams(p);
}

const side = { on_target: 0, above: 1, below: 2 } as const;
type Cmp = (a: Decided, b: Decided) => number;
const desc = (f: (d: Decided) => number | undefined): Cmp => (a, b) => (f(b) ?? -Infinity) - (f(a) ?? -Infinity);
const comparators: Record<Exclude<SortKey, 'automation'>, Cmp[]> = {
  // Closest first; equally close: on target, then above (meets the cutoff), then below; outside the profile last.
  desirability: [desc((d) => d.decision.desirability?.score), (a, b) =>
    (side[a.decision.desirability?.position ?? 'below'] ?? 3) - (side[b.decision.desirability?.position ?? 'below'] ?? 3)],
  seeders: [desc((d) => (d.release.protocol === 'torrent' ? d.release.seeders : undefined))],
  size: [desc((d) => d.release.size)],
  newest: [desc((d) => Date.parse(d.release.publishedAt) || undefined)],
  quality: [desc((d) => d.decision.desirability?.qualitySteps)],
};

/** Orders releases by a sort key; ties keep automation order. */
export function sortReleases(list: Decided[], key: SortKey): Decided[] {
  const ranked = rank(list);
  if (key === 'automation') return ranked;
  const pos = new Map(ranked.map((d, i) => [d, i]));
  const cmps = comparators[key];
  return ranked.slice().sort((a, b) => {
    for (const c of cmps) {
      const r = c(a, b);
      if (r) return r;
    }
    return pos.get(a)! - pos.get(b)!;
  });
}

/** What automation would do with one release: pending while streaming, unavailable if the search stopped early. */
export function outcomeFor(d: Decided, verdict: AutomationVerdict | null | undefined, streaming: boolean): AutomationOutcome {
  const o = verdict?.outcomes?.[d.release.id];
  if (o) return o.status;
  if (!d.decision.accepted) return 'declined';
  return streaming ? 'pending' : 'unavailable';
}

/** Context a panel needs beyond the release itself. */
export interface PanelContext { verdict?: AutomationVerdict | null; streaming?: boolean; runtimeSeconds?: number }

/** One decided release as the decision panel shows it. */
export function toPanel(d: Decided, i: number, accepted: number, ctx: PanelContext | number = {}): PanelDecision {
  const c: PanelContext = typeof ctx === 'number' ? { runtimeSeconds: ctx } : ctx;
  const r = d.release;
  const outcome = outcomeFor(d, c.verdict, c.streaming ?? false);
  return {
    release: {
      title: r.title, indexer: (r.indexers ?? []).map((x) => x.name || x.key).join(', ') || 'unknown indexer', protocol: r.protocol,
      size: r.size, publishedAt: r.publishedAt, seeders: r.protocol === 'torrent' ? r.seeders : undefined, quality: d.decision.quality || 'unknown',
      runtimeSeconds: c.runtimeSeconds, swarm: d.swarm ?? undefined, availability: d.availability ?? undefined,
    },
    verdict: d.decision.accepted ? 'accept' : 'reject',
    rank: d.decision.accepted ? i + 1 : undefined,
    candidates: d.decision.accepted ? accepted : undefined,
    score: { total: d.decision.formatScore, formats: [] },
    reasons: (d.decision.reasons ?? []).map<DecisionReason>((x) => ({ rule: x.ruleId, category: x.category, message: x.message, summary: x.summary, blocklist: x.ruleId.includes('blocklist') })),
    automation: { outcome, lostTo: c.verdict?.outcomes?.[r.id]?.lostTo?.message },
    desirability: d.decision.desirability ?? null,
  };
}

/** The note shown when nothing passed: what automation would grab (nothing) and the main reasons. */
export function nothingAcceptedNote(total: number, rejections: { message: string; count: number }[] | null | undefined): string {
  const top = (rejections ?? []).slice(0, 3).map((r) => `${r.message} (${r.count})`);
  return `Automatic search would grab nothing: none of the ${total} releases passed the profile.` +
    (top.length ? ` Most common reasons: ${top.join('; ')}.` : '') + ' You can grab one anyway where the rules allow it.';
}

/** Reads a grab failure: 409 needs confirmation, 403 is refused, the rest failed. */
export function grabFailure(e: unknown): GrabFailure {
  const x = toApiError(e);
  const reasons = ((x.body.decision as { reasons?: { message: string }[] } | undefined)?.reasons ?? []).map((r) => r.message);
  const message = x.status === 404 ? 'The search results expired. Run the search again.' : x.message;
  return { confirm: x.status === 409 && x.code === 'confirm_required', refused: x.status === 403, message, reasons };
}

export class InteractiveSearch {
  decided = $state<Decided[]>([]);
  sources = $state<SourceProgress[]>([]);
  error = $state<ApiError | undefined>();
  streaming = $state(false);
  summary = $state<Summary | undefined>();
  sort = $state<SortKey>(readSort());
  readonly #api: Client;
  #ctl: AbortController | undefined;

  constructor(api: Client) {
    this.#api = api;
  }

  get ranked(): Decided[] { return rank(this.decided); }
  get sorted(): Decided[] { return sortReleases(this.decided, this.sort); }
  get verdict(): AutomationVerdict | null | undefined { return this.summary?.automation; }

  /** Changes the order and keeps it in the URL. */
  setSort(key: SortKey): void {
    this.sort = key;
    writeSort(key);
  }

  /** A panel for one release, ranked in automation order. */
  panel(d: Decided, runtimeSeconds?: number): PanelDecision {
    return toPanel(d, this.ranked.indexOf(d), this.accepted, { verdict: this.verdict, streaming: this.streaming, runtimeSeconds });
  }
  get accepted(): number { return this.decided.filter((d) => d.decision.accepted).length; }

  /** Streams the decisions of one search path (".../releases"). */
  async start(path: string): Promise<void> {
    this.stop();
    const ctl = new AbortController();
    this.#ctl = ctl;
    this.decided = [];
    this.sources = [];
    this.summary = undefined;
    this.error = undefined;
    this.streaming = true;
    try {
      for await (const line of this.#api.stream<ReleaseLine>(path, ctl.signal)) this.#take(line);
    } catch (e) {
      if (!ctl.signal.aborted) this.error = toApiError(e);
    } finally {
      if (this.#ctl === ctl) this.streaming = false;
    }
  }

  #take(line: ReleaseLine): void {
    if ('record' in line) {
      this.decided = [...this.decided, line.record];
      for (const ix of line.record.release.indexers ?? []) this.#source({ id: ix.key, name: ix.name || ix.key, state: 'ok' }, 1);
    } else if ('error' in line) {
      this.#source({ id: line.error.source, name: line.error.source, state: 'failed', reason: line.error.message }, 0);
    } else {
      this.summary = line.summary;
    }
  }

  #source(s: SourceProgress, add: number): void {
    const i = this.sources.findIndex((x) => x.id === s.id);
    const count = (i < 0 ? 0 : (this.sources[i].count ?? 0)) + add;
    const next = { ...s, count: s.state === 'ok' ? count : undefined };
    this.sources = i < 0 ? [...this.sources, next] : this.sources.map((x, j) => (j === i ? next : x));
  }

  stop(): void {
    this.#ctl?.abort();
    this.streaming = false;
  }
}
