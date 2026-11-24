// Interactive search state for a drawer: streams NDJSON decisions as indexers answer, keeps
// per-indexer progress, ranks the releases best first and turns each into a DecisionPanel
// decision. A grab goes through the override rules: 409 asks to confirm, 403 refuses.
import type { SourceProgress } from '../lib/components/molecules/SourceProgressStrip/SourceProgressStrip.svelte';
import type { Decision as PanelDecision, DecisionReason } from '../lib/components/organisms/DecisionPanel/DecisionPanel.svelte';
import { ApiError, toApiError, type Client } from './api/client';
import type { Decided, ReleaseLine } from './api/types';

type Summary = Extract<ReleaseLine, { summary: unknown }>['summary'];

/** Why a grab did not go through. */
export interface GrabFailure { confirm: boolean; refused: boolean; message: string; reasons: string[] }

/** Ranks decisions: accepted first, then by rank key, best first. */
export function rank(list: Decided[]): Decided[] {
  return list.slice().sort((a, b) =>
    Number(b.decision.accepted) - Number(a.decision.accepted) || (a.decision.rankKey < b.decision.rankKey ? 1 : a.decision.rankKey > b.decision.rankKey ? -1 : 0));
}

/** One decided release as the decision panel shows it. */
export function toPanel(d: Decided, i: number, accepted: number, runtimeSeconds?: number): PanelDecision {
  const r = d.release;
  return {
    release: {
      title: r.title, indexer: (r.indexers ?? []).map((x) => x.name || x.key).join(', ') || 'unknown indexer', protocol: r.protocol,
      size: r.size, publishedAt: r.publishedAt, seeders: r.protocol === 'torrent' ? r.seeders : undefined, quality: d.decision.quality || 'unknown',
      runtimeSeconds,
    },
    verdict: d.decision.accepted ? 'accept' : 'reject',
    rank: d.decision.accepted ? i + 1 : undefined,
    candidates: d.decision.accepted ? accepted : undefined,
    score: { total: d.decision.formatScore, formats: [] },
    reasons: (d.decision.reasons ?? []).map<DecisionReason>((x) => ({ rule: x.ruleId, category: x.category, message: x.message, blocklist: x.ruleId.includes('blocklist') })),
  };
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
  readonly #api: Client;
  #ctl: AbortController | undefined;

  constructor(api: Client) {
    this.#api = api;
  }

  get ranked(): Decided[] { return rank(this.decided); }
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
