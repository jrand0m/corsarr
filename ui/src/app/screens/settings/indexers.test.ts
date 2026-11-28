import { describe, expect, it } from 'vitest';
import type { DefinitionEntry, IndexerRow, PluginManifest } from '../../api/types';
import {
  addSchema, editView, fieldErrors, pluginChoices, problemLines, refusedTest, rowStatus, suggestKey, testChecks, toEdit, toInput, torznabURL,
} from './indexers';

const manifest = (id: string, delivery: string, settings: PluginManifest['settings'] = []): PluginManifest =>
  ({ id, kind: 'indexer', version: '1', apiVersion: 1, name: id === 'torznab' ? 'Torznab' : id.toUpperCase(), delivery, settings });

const row = (over: Partial<IndexerRow> = {}): IndexerRow => ({
  key: 'tpb', name: 'TPB', plugin: 'thepiratebay', pluginName: 'The Pirate Bay', delivery: 'declarative', source: 'ui', enabled: true,
  running: true, priority: 25, settings: { baseUrl: 'https://apibay.org/', priority: 25 }, secretsSet: [], torznab: '/torznab/tpb/api', ...over,
});

describe('testChecks', () => {
  it('reports a failed connection with its class only', () => {
    const c = testChecks({ ok: false, class: 'session_expired', message: 'logged out', elapsedMs: 40 });
    expect(c).toEqual([{ id: 'connect', name: 'Connection and sign-in', status: 'fail', ms: 40, detail: 'logged out (session expired)' }]);
  });
  it('summarises what the indexer can search', () => {
    const c = testChecks({ ok: true, message: 'connected', elapsedMs: 5, caps: { modes: ['search', 'movie'], idParams: ['imdbid'], categories: { 2000: 'Movies' }, limit: 100 } });
    expect(c.map((x) => [x.id, x.status, x.detail])).toEqual([
      ['connect', 'pass', undefined], ['modes', 'pass', 'text, movies'], ['ids', 'pass', 'imdbid'], ['cats', 'pass', '1 mapped'],
    ]);
  });
  it('flags empty capabilities', () => {
    const c = testChecks({ ok: true, message: '', elapsedMs: 1, caps: { modes: null, idParams: null, categories: null, limit: 0 } });
    expect(c.slice(1).map((x) => x.status)).toEqual(['warn', 'skipped', 'warn']);
    expect(testChecks({ ok: false, message: 'down', elapsedMs: 1 })[0].detail).toBe('down');
  });
  it('names known search modes and shows unknown ones as declared', () => {
    const c = testChecks({ ok: true, message: '', elapsedMs: 1, caps: { modes: ['tv', 'music', 'book', 'audiobook'], idParams: [], categories: {}, limit: 0 } });
    expect(c[1]).toMatchObject({ status: 'pass', detail: 'TV, music, books, audiobook' });
    expect(c.slice(2).map((x) => [x.status, x.detail])).toEqual([
      ['skipped', 'Text searches only'], ['warn', 'No categories; every search covers all of them'],
    ]);
  });
});

describe('pluginChoices', () => {
  it('lists indexers only, built-ins first, with definition facts', () => {
    const defs: DefinitionEntry[] = [
      { id: 'zz', name: 'ZZ', origin: 'user', file: 'zz.yaml', valid: true, privacy: 'public', language: 'en' },
      { id: 'bad', name: 'Bad', origin: 'user', file: 'bad.yaml', valid: false },
    ];
    const got = pluginChoices([manifest('zz', 'declarative'), manifest('torznab', 'builtin'), { ...manifest('qbit', 'builtin'), kind: 'downloader' }, manifest('aa', 'declarative')], defs);
    expect(got).toEqual([
      { value: 'torznab', label: 'Torznab', description: 'Built in' },
      { value: 'aa', label: 'AA', description: 'Definition' },
      { value: 'zz', label: 'ZZ', description: 'Definition · public · en · your file' },
    ]);
  });
});

describe('keys and inputs', () => {
  it('suggests free keys', () => {
    expect(suggestKey('The Pirate Bay', [])).toBe('thepiratebay');
    expect(suggestKey('The Pirate Bay', ['thepiratebay', 'thepiratebay2'])).toBe('thepiratebay3');
    expect(suggestKey('123', [])).toBe('indexer');
  });
  it('puts key and name before the plugin settings', () => {
    const s = addSchema(manifest('x', 'declarative', [{ key: 'baseUrl', type: 'url', label: 'Site' }]));
    expect(s.fields.map((f) => f.key)).toEqual(['key', 'name', 'baseUrl']);
    expect(s.fields[0].validation?.pattern).toBe('^[a-z][a-z0-9]*$');
  });
  it('splits form values into requests', () => {
    expect(toInput('tpb', { key: 'a', name: 'A', baseUrl: 'u' })).toEqual({ key: 'a', plugin: 'tpb', name: 'A', settings: { baseUrl: 'u' } });
    expect(toInput('tpb', { key: 'a' })).toEqual({ key: 'a', plugin: 'tpb', name: undefined, settings: {} });
    expect(toEdit({ name: 'B' })).toEqual({ name: 'B' });
    expect(toEdit({ priority: 3 })).toEqual({ settings: { priority: 3 } });
  });
});

describe('editView', () => {
  const m = manifest('thepiratebay', 'declarative', [{ key: 'baseUrl', type: 'url', label: 'Site' }, { key: 'cookie', type: 'secret', label: 'Cookie' }]);
  it('fills values and marks stored secrets', () => {
    const v = editView(row({ secretsSet: ['cookie'] }), m);
    expect(v.values).toEqual({ name: 'TPB', baseUrl: 'https://apibay.org/' });
    expect(v.meta).toEqual({ cookie: { isSet: true, managed: undefined } });
  });
  it('locks config indexers to their keys', () => {
    const v = editView(row({ source: 'config', key: 'k' }), m);
    expect(v.meta.name.managed).toBe('indexers.k.name');
    expect(v.meta.baseUrl.managed).toBe('indexers.k.baseUrl');
  });
});

describe('server messages', () => {
  it('maps refused fields and tests', () => {
    expect(fieldErrors({ fields: [{ field: 'baseUrl', message: 'must be an http(s) address' }] })).toEqual({ baseUrl: 'must be an http(s) address.' });
    expect(fieldErrors(undefined)).toEqual({});
    expect(refusedTest({ test: { ok: false, message: 'x', elapsedMs: 1 } })?.message).toBe('x');
  });
  it('lists definition problems', () => {
    const e: DefinitionEntry = { id: 'b', name: 'b', origin: 'user', file: 'b.yaml', valid: false,
      problems: [{ path: 'id', line: 3, message: 'bad id' }, { path: '', line: 0, message: 'not yaml' }] };
    expect(problemLines(e)).toEqual(['id (line 3): bad id', 'file: not yaml']);
  });
  it('derives row status and addresses', () => {
    expect(rowStatus(row({ enabled: false }))).toBe('disabled');
    expect(rowStatus(row())).toBeUndefined();
    expect(torznabURL('/torznab/all/api', 'http://box:8989/')).toBe('http://box:8989/torznab/all/api');
  });
});
