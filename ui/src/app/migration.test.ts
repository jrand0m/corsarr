import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { planAsJSON, planAsText,
  addressReady, applyCount, configNotice, countTiles, defaultForm, fromPrefixView, hasConfigNotice, InstantCheck, keyReady, pathCheckBody, pathCheckReady,
  pathCheckSig, sampleView, sourceCheckBody, sourceCheckReady, sourceCheckSig, sourceCheckView, toPrefixView, type CheckView, type KindState, type PathCheck,
  type SourceCheck, filterEntries, outcomeView, planFacets, progressOf, requestBody, scopeProblems, sourceProblems, toggleExclude,
  writes, type PlanEntry, type SourceApp, type StreamLine,
} from './migration';
import { vocabulary } from '../lib/status/vocabulary';

const entries: PlanEntry[] = [
  { id: 'sonarr:series:1', category: 'library', source: 'sonarr', name: 'Show (2020)', outcome: 'create', reason: 'new' },
  { id: 'sonarr:episodefile:2', category: 'files', source: 'sonarr', name: '/tv/a.mkv', outcome: 'fileMissing', reason: 'not found on disk' },
  { id: 'corsarr:rules:all', category: 'modes', source: '', name: 'Automation rules', outcome: 'create', reason: 'dry-run' },
];

describe('migration form', () => {
  it('offers every supported source app', () => {
    expect(defaultForm().sources.map((s) => s.app)).toEqual(['sonarr', 'radarr', 'lidarr', 'prowlarr']);
  });

  it('builds the request from the chosen sources only', () => {
    const f = defaultForm();
    f.sources[0] = { app: 'sonarr', include: true, url: ' http://sonarr:8989 ', apiKey: 'k' };
    f.sources[1] = { app: 'radarr', include: false, url: 'http://radarr', apiKey: 'k' };
    f.fromPrefix = '/data/media';
    f.toPrefix = '/srv/media';
    expect(requestBody(f, 'abc')).toEqual({
      sources: [{ app: 'sonarr', url: 'http://sonarr:8989', apiKey: 'k' }], scope: f.scope, activate: false, exclude: [],
      pathRewrite: '/data/media:/srv/media', planHash: 'abc',
    });
    f.scope = [...f.scope, 'history'];
    expect(requestBody(f).historyDays).toBe(90);
  });

  it('checks sources and scope before moving on', () => {
    const f = defaultForm();
    expect(sourceProblems(f)).toEqual(['Choose at least one source.']);
    f.sources[3] = { app: 'prowlarr', include: true, url: 'prowlarr', apiKey: '' };
    expect(sourceProblems(f)).toHaveLength(2);
    f.sources[3] = { app: 'prowlarr', include: true, url: 'https://p', apiKey: 'k' };
    expect(sourceProblems(f)).toEqual([]);
    expect(scopeProblems(f)).toEqual([]);
    f.sources[2] = { app: 'lidarr', include: true, url: 'http://lidarr:8686', apiKey: '' };
    expect(sourceProblems(f)).toEqual(['Lidarr: enter its API key.']);
    f.fromPrefix = 'data';
    expect(scopeProblems(f)).toHaveLength(2);
    f.scope = [];
    f.fromPrefix = '';
    expect(scopeProblems(f)).toEqual(['Choose at least one category.']);
  });
});

describe('migration report', () => {
  it('shows every outcome with a known badge', () => {
    for (const o of ['create', 'update', 'unchanged', 'skip', 'approximate', 'unsupported', 'conflict', 'secretNeeded', 'fileMissing',
      'managedByConfig', 'gone', 'failed'] as const) {
      const v = outcomeView(o);
      expect((vocabulary as Record<string, Record<string, unknown>>)[v.family][v.value], o).toBeDefined();
    }
  });

  it('orders the count tiles and drops empty ones', () => {
    expect(countTiles({ skip: 1, create: 3, update: 0 })).toEqual([{ outcome: 'create', label: 'Create', n: 3 }, { outcome: 'skip', label: 'Skip', n: 1 }]);
    expect(countTiles(null)).toEqual([]);
  });

  it('filters by facet and text', () => {
    const facets = planFacets(entries);
    expect(facets.map((f) => f.id)).toEqual(['outcome', 'category', 'source']);
    expect(facets[2].options.map((o) => o.value)).toEqual(['corsarr', 'sonarr']);
    expect(facets[0].options.map((o) => [o.value, o.count])).toEqual([['create', 2], ['fileMissing', 1]]);
    expect(facets[2].options.map((o) => [o.label, o.count])).toEqual([['corsarr', 1], ['Sonarr', 2]]);
    expect(filterEntries(entries, { q: '', facets: { outcome: ['create'] } })).toHaveLength(2);
    expect(filterEntries(entries, { q: 'disk', facets: {} })).toEqual([entries[1]]);
    expect(filterEntries(entries, { q: '', facets: { source: ['corsarr'] } })).toEqual([entries[2]]);
  });

  it('tracks progress per source from the stream', () => {
    const running: StreamLine[] = [{ record: { progress: { source: 'sonarr', step: 'series', done: 0, total: 0 } } }];
    expect(progressOf(['sonarr', 'radarr'], running).map((p) => p.state)).toEqual(['running', 'pending']);
    const done: StreamLine[] = [...running, { summary: { done: true, report: { mode: 'plan', planHash: 'h', counts: {}, entries,
      sources: [{ app: 'sonarr', url: 'u', supported: '3.x' }, { app: 'radarr', url: 'u', supported: '3.x', error: 'unauthorised' }] } } }];
    const p = progressOf(['sonarr', 'radarr'], done);
    expect(p[0]).toMatchObject({ state: 'ok', count: 2 });
    expect(p[1]).toMatchObject({ state: 'failed', reason: 'unauthorised' });
  });

  it('counts no entries for a report without them and names an unknown source by its id', () => {
    const app = 'mylar' as SourceApp;
    const lines: StreamLine[] = [{ summary: { done: true, report: { mode: 'plan', planHash: 'h', counts: {}, entries: undefined as unknown as PlanEntry[], sources: [] } } }];
    expect(progressOf(['sonarr', app], lines)).toEqual([
      { id: 'sonarr', name: 'Sonarr', state: 'ok', count: 0 },
      { id: 'mylar', name: 'mylar', state: 'ok', count: 0 },
    ]);
    expect(progressOf([app], [])).toEqual([{ id: 'mylar', name: 'mylar', state: 'pending' }]);
    const noSources: StreamLine[] = [{ summary: { done: true, report: { mode: 'plan', planHash: 'h', counts: {}, entries, sources: null as unknown as [] } } }];
    expect(progressOf(['sonarr'], noSources)).toEqual([{ id: 'sonarr', name: 'Sonarr', state: 'ok', count: 2 }]);
  });

  it('leaves out only what an apply would write', () => {
    expect(entries.filter(writes).map((e) => e.id)).toEqual(['sonarr:series:1', 'corsarr:rules:all']);
    expect(toggleExclude(toggleExclude([], 'a'), 'a')).toEqual([]);
    const profile: PlanEntry = { id: 'sonarr:profile:2', category: 'profiles', source: 'sonarr', name: 'Top', outcome: 'approximate', reason: 'x' };
    expect(writes(profile)).toBe(false);
    expect(applyCount([...entries, profile], ['corsarr:rules:all'])).toBe(1);
    expect(applyCount(null, [])).toBe(0);
  });
});

/** A check whose runs resolve only when the test says so. */
function rig(delay = 600) {
  const runs: { input: string; signal: AbortSignal; resolve: (r: string) => void; reject: (e: unknown) => void }[] = [];
  const views: CheckView<string>[] = [];
  const check = new InstantCheck<string, string>({
    run: (input, signal) => new Promise((resolve, reject) => runs.push({ input, signal, resolve, reject })),
    sig: (s) => s.trim(),
    ready: (s) => s.trim().length >= 3,
    onview: (v) => views.push(v),
    delay,
  });
  const last = () => views[views.length - 1];
  return { check, runs, views, last };
}

describe('instant check', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('waits for typing to pause, then checks once', async () => {
    const r = rig();
    r.check.input('ab');
    r.check.input('abc');
    vi.advanceTimersByTime(599);
    expect(r.runs).toHaveLength(0);
    r.check.input('abcd');
    vi.advanceTimersByTime(599);
    expect(r.runs).toHaveLength(0);
    vi.advanceTimersByTime(1);
    expect(r.runs.map((x) => x.input)).toEqual(['abcd']);
    expect(r.last()).toEqual({ status: 'checking' });
    r.runs[0].resolve('ok');
    await Promise.resolve();
    expect(r.last()).toEqual({ status: 'done', result: 'ok' });
  });

  it('checks a paste at once and does not wait for the pause', () => {
    const r = rig();
    r.check.paste();
    r.check.input('pasted');
    expect(r.runs.map((x) => x.input)).toEqual(['pasted']);
    vi.advanceTimersByTime(1000);
    expect(r.runs).toHaveLength(1);
  });

  it('does not check incomplete input, and a paste flag does not outlive its input', () => {
    const r = rig();
    r.check.paste();
    r.check.input('ab');
    r.check.input('abc');
    expect(r.runs).toHaveLength(0);
    vi.advanceTimersByTime(600);
    expect(r.runs).toHaveLength(1);
  });

  it('cancels the in-flight check when the input changes and never shows its answer', async () => {
    const r = rig();
    r.check.blur('first');
    const first = r.runs[0];
    r.check.input('second');
    expect(first.signal.aborted).toBe(true);
    expect(r.last()).toEqual({ status: 'idle' });
    first.resolve('stale');
    await Promise.resolve();
    expect(r.views.some((v) => v.result === 'stale')).toBe(false);
    vi.advanceTimersByTime(600);
    r.runs[1].resolve('fresh');
    await Promise.resolve();
    expect(r.last()).toEqual({ status: 'done', result: 'fresh' });
  });

  it('never runs the same check twice at once, and blur does not repeat a finished one', async () => {
    const r = rig();
    r.check.blur('same');
    r.check.blur('same ');
    r.check.test('same');
    expect(r.runs).toHaveLength(1);
    r.check.input('same');
    expect(r.runs[0].signal.aborted).toBe(false);
    r.runs[0].resolve('ok');
    await Promise.resolve();
    r.check.blur('same');
    vi.advanceTimersByTime(600);
    expect(r.runs).toHaveLength(1);
    expect(r.check.test('same')).toBe(true);
    expect(r.runs).toHaveLength(2);
  });

  it('a blur pre-empts the pending pause; Test refuses incomplete input', () => {
    const r = rig();
    r.check.input('typed');
    r.check.blur('typed');
    vi.advanceTimersByTime(600);
    expect(r.runs).toHaveLength(1);
    expect(r.check.test('no')).toBe(false);
    r.check.blur('no');
    expect(r.runs).toHaveLength(1);
  });

  it('shows a failure to ask, and cancel stops everything', async () => {
    const r = rig();
    r.check.blur('boom');
    r.runs[0].reject(new Error('network down'));
    await Promise.resolve();
    await Promise.resolve();
    expect(r.last()).toEqual({ status: 'error', error: 'network down' });
    r.check.test('again');
    r.runs[1].reject('plain');
    await Promise.resolve();
    await Promise.resolve();
    expect(r.last()).toEqual({ status: 'error', error: 'plain' });
    r.check.input('pending');
    r.check.test('other');
    r.check.cancel();
    expect(r.runs[2].signal.aborted).toBe(true);
    vi.advanceTimersByTime(600);
    expect(r.runs).toHaveLength(3);
  });
});

describe('source check', () => {
  const full = { app: 'sonarr' as const, url: ' http://sonarr:8989 ', apiKey: ' abc123 ', withKey: true };

  it('checks only well-formed addresses and keys', () => {
    expect(addressReady('http://sonarr:8989')).toBe(true);
    expect(addressReady('sonarr:8989')).toBe(false);
    expect(addressReady('ftp://x')).toBe(false);
    expect(addressReady('')).toBe(false);
    expect(keyReady(' k ')).toBe(true);
    expect(keyReady('a b')).toBe(false);
    expect(sourceCheckReady(full)).toBe(true);
    expect(sourceCheckReady({ ...full, apiKey: '' })).toBe(false);
    expect(sourceCheckReady({ ...full, apiKey: '', withKey: false })).toBe(true);
  });

  it('keeps the key out of the signature and out of an address-only request', () => {
    const sig = sourceCheckSig(full);
    expect(sig).not.toContain('abc123');
    expect(sig).toBe(sourceCheckSig({ ...full, apiKey: 'abc123' }));
    expect(sig).not.toBe(sourceCheckSig({ ...full, apiKey: 'abc124' }));
    expect(sourceCheckSig({ ...full, withKey: false })).toBe(sourceCheckSig({ ...full, apiKey: 'other', withKey: false }));
    expect(sourceCheckBody(full)).toEqual({ app: 'sonarr', url: 'http://sonarr:8989', apiKey: 'abc123' });
    expect(sourceCheckBody({ ...full, withKey: false })).toEqual({ app: 'sonarr', url: 'http://sonarr:8989' });
  });

  it('describes every outcome with a known badge', () => {
    const base: SourceCheck = { app: 'sonarr', state: 'reachable', name: 'Sonarr', version: '4.0.20', supported: '3.x to 4.x' };
    expect(sourceCheckView({ status: 'idle' })).toBeUndefined();
    expect(sourceCheckView({ status: 'checking' })).toEqual({ value: 'checking' });
    expect(sourceCheckView({ status: 'done', result: base })).toEqual({ value: 'reachable', detail: 'Sonarr 4.0.20 · supported 3.x to 4.x' });
    expect(sourceCheckView({ status: 'done', result: { ...base, name: '', version: undefined } })!.detail).toBe('Sonarr · supported 3.x to 4.x');
    expect(sourceCheckView({ status: 'error', error: 'HTTP 500' })).toMatchObject({ value: 'unreachable' });
    for (const state of ['keyNeeded', 'unauthorised', 'wrongApp', 'unsupported', 'unreachable'] as const) {
      const v = sourceCheckView({ status: 'done', result: { ...base, state, reason: 'why' } })!;
      expect(vocabulary.sourceCheck[v.value as keyof typeof vocabulary.sourceCheck], state).toBeDefined();
      expect(v.detail).toBe('why');
    }
  });

  it('explains a check that got no answer and names an unknown app by its id', () => {
    expect(sourceCheckView({ status: 'error' })).toEqual({ value: 'unreachable', detail: 'corsarr could not run the check: no answer' });
    expect(sourceCheckView({ status: 'done' })).toEqual({ value: 'unreachable', detail: 'corsarr could not run the check: no answer' });
    expect(sourceCheckView({ status: 'error', error: '' })).toEqual({ value: 'unreachable', detail: 'corsarr could not run the check: ' });
    const r: SourceCheck = { app: 'mylar' as SourceApp, state: 'reachable', version: '1.2', supported: '1.x' };
    expect(sourceCheckView({ status: 'done', result: r })).toEqual({ value: 'reachable', detail: 'mylar 1.2 · supported 1.x' });
  });
});

describe('path check', () => {
  const form = defaultForm();
  form.sources[0] = { app: 'sonarr', include: true, url: 'http://sonarr:8989', apiKey: 'k' };
  form.sources[3] = { app: 'prowlarr', include: true, url: 'http://p', apiKey: 'k' };
  const roots = (over: Partial<PathCheck> = {}): PathCheck => ({
    roots: [
      { source: 'sonarr', path: '/data/media/tv', matched: true, rewritten: '/media/tv', exists: true, readable: true, writable: false },
      { source: 'sonarr', path: '/other', matched: false, rewritten: '/other', exists: false, readable: false, writable: false },
    ],
    target: { path: '/media', exists: true, readable: true, writable: false },
    sources: [],
    ...over,
  });

  it('needs a readable source and absolute or empty prefixes', () => {
    const i = { sources: form.sources, fromPrefix: '', toPrefix: '' };
    expect(pathCheckReady(i)).toBe(true);
    expect(pathCheckReady({ ...i, fromPrefix: 'data' })).toBe(false);
    expect(pathCheckReady({ ...i, sources: defaultForm().sources })).toBe(false);
    expect(pathCheckBody({ ...i, fromPrefix: ' /data ' })).toEqual({
      sources: [{ app: 'sonarr', url: 'http://sonarr:8989', apiKey: 'k' }], fromPrefix: '/data', toPrefix: '',
    });
    expect(pathCheckSig(i)).not.toBe(pathCheckSig({ ...i, toPrefix: '/x' }));
    expect(pathCheckSig(i)).not.toContain('\nk');
  });

  it('reports matched and unmapped roots for the source prefix', () => {
    expect(fromPrefixView(roots(), '')).toMatchObject({ value: 'pass' });
    expect(fromPrefixView(roots(), '/data/media')).toEqual({
      value: 'warn', detail: 'Matches 1 of 2 root folders.', items: ['Matches /data/media/tv', 'Unmapped /other'],
    });
    const none = roots();
    none.roots = none.roots.map((r) => ({ ...r, matched: false }));
    expect(fromPrefixView(none, '/nope').value).toBe('fail');
    const all = roots();
    all.roots = [all.roots[0]];
    expect(fromPrefixView(all, '/data/media')).toMatchObject({ value: 'pass', detail: 'Matches every root folder (1).' });
    expect(fromPrefixView(roots({ roots: [] }), '/x').value).toBe('warn');
  });

  it('reports the corsarr prefix: exists and readable decide, writable is information', () => {
    expect(toPrefixView(roots(), '')).toMatchObject({ value: 'pass', detail: 'No rewrite.' });
    expect(toPrefixView(roots(), '/media').detail).toMatch(/Read-only, which is fine/);
    expect(toPrefixView(roots({ target: { path: '/media', exists: true, readable: true, writable: true } }), '/media').detail).toMatch(/writable/);
    expect(toPrefixView(roots({ target: { path: '/m', exists: false, readable: false, writable: false } }), '/m').value).toBe('fail');
    expect(toPrefixView(roots({ target: { path: '/m', exists: true, readable: false, writable: false } }), '/m').detail).toMatch(/cannot read/);
  });

  it('samples each root with its rewritten path and whether it exists', () => {
    const s = sampleView(roots({ sources: [{ app: 'radarr', reason: 'The app refused this API key.' }] }));
    expect(s.value).toBe('warn');
    expect(s.items).toEqual(['/data/media/tv → /media/tv: found', '/other: missing', 'Radarr: The app refused this API key.']);
    const ok = roots();
    ok.roots = [ok.roots[0]];
    expect(sampleView(ok)).toMatchObject({ value: 'pass', detail: 'All 1 root folders exist for corsarr.' });
    const bad = roots();
    bad.roots = [bad.roots[1], { ...bad.roots[0], readable: false }];
    expect(sampleView(bad).value).toBe('fail');
    expect(sampleView(bad).items![1]).toMatch(/found, not readable/);
    expect(sampleView(roots({ roots: [] })).value).toBe('warn');
  });

  it('warns about unreadable sources even when every root is found', () => {
    const ok = roots({ sources: [{ app: 'mylar' as SourceApp, reason: 'Unreachable.' }] });
    ok.roots = [ok.roots[0]];
    expect(sampleView(ok)).toEqual({
      value: 'warn', detail: 'All 1 root folders exist for corsarr.', items: ['/data/media/tv → /media/tv: found', 'mylar: Unreachable.'],
    });
  });
});

describe('config-managed notice', () => {
  const modes: KindState[] = [
    { kind: 'tv', mode: 'active', managedByConfig: true },
    { kind: 'movie', mode: 'active', managedByConfig: false },
    { kind: 'music', mode: 'active', managedByConfig: true },
  ];

  it('leaves out a managed kind the config already sets to the wanted mode', () => {
    const f = defaultForm();
    f.sources[0].include = true;
    expect(configNotice(f, [{ kind: 'tv', mode: 'shadow', managedByConfig: true }]).kinds).toEqual([]);
    f.activate = true;
    expect(configNotice(f, modes).kinds).toEqual([]);
  });

  it('names managed kinds of the chosen sources with the snippet, and what still imports', () => {
    const f = defaultForm();
    f.sources[0].include = true;
    f.sources[1].include = true;
    const n = configNotice(f, modes);
    expect(n.kinds).toEqual([{ kind: 'tv', label: 'TV', snippet: 'mediaKinds:\n  tv:\n    mode: shadow\n' }]);
    expect(n.snippetOnly).toEqual(['Root folders', 'Indexers', 'Download clients', 'Naming']);
    expect(n.importing).toEqual(['Library', 'Profiles', 'In-flight downloads', 'Blocklist']);
    expect(n.nothingWritten).toBe(false);
    expect(hasConfigNotice(n)).toBe(true);
  });

  it('warns when every chosen category is config-only, and stays quiet when nothing is managed', () => {
    const f = defaultForm();
    f.sources[0].include = true;
    f.scope = ['indexers', 'clients'];
    const n = configNotice(f, modes);
    expect(n.kinds).toEqual([]);
    expect(n.nothingWritten).toBe(true);
    f.scope = ['library', 'history'];
    const quiet = configNotice(f, [{ kind: 'tv', mode: 'shadow', managedByConfig: false }]);
    expect(hasConfigNotice(quiet)).toBe(false);
    expect(configNotice({ ...f, scope: [] }, modes).nothingWritten).toBe(false);
    expect(configNotice({ ...f, scope: ['naming'] }, modes).nothingWritten).toBe(true);
  });
});

describe('plan export', () => {
  const rows = [
    { id: 'a', category: 'library', source: 'sonarr', name: 'Severance', outcome: 'create' as const, reason: 'new\tseries', target: 'tv:1' },
    { id: 'b', category: 'profiles', source: 'radarr', name: 'HD-1080p', outcome: 'approximate' as const, reason: 'mapped to hd' },
  ];
  it('copies the shown rows as tab-separated text with a header', () => {
    const lines = planAsText(rows).trimEnd().split('\n');
    expect(lines[0]).toBe('Outcome\tCategory\tSource\tObject\tReason\tTarget');
    expect(lines).toHaveLength(3);
    expect(lines[1].split('\t')).toHaveLength(6);
    expect(lines[1]).toContain('new series');
  });
  it('copies the shown rows as JSON', () => {
    expect(JSON.parse(planAsJSON(rows))).toEqual(rows);
  });
});
