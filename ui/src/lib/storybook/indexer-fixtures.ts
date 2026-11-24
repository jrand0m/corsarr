// Fixtures for the Settings › Indexers screen stories: indexers from config and the UI,
// the indexer plugins of a build, and a definition catalogue with one broken file.
import type { DefinitionEntry, IndexerRow, IndexerTest, PluginManifest } from '../../app/api/types';

const since = '2026-11-22T13:00:00Z';

export const indexerManifests: PluginManifest[] = [
  { id: 'torznab', kind: 'indexer', version: '1.0.0', apiVersion: 1, name: 'Torznab', delivery: 'builtin', settings: [
    { key: 'url', type: 'url', label: 'Indexer URL', required: true, help: 'Base address of the Torznab indexer.' },
    { key: 'apiPath', type: 'text', label: 'API path', default: '/api', help: 'Path appended to the address.' },
    { key: 'apiKey', type: 'secret', label: 'API key', help: 'API key issued by the indexer.' },
    { key: 'priority', type: 'number', label: 'Priority', default: 25, help: 'Lower numbers win ties (1 to 50).' },
  ] },
  { id: 'knaben', kind: 'indexer', version: '1', apiVersion: 1, name: 'Knaben', delivery: 'declarative', settings: [
    { key: 'baseUrl', type: 'url', label: 'Site address', default: 'https://api.knaben.org/', help: 'Address of the site. Change it only when the site moves.' },
    { key: 'hideUnsafe', type: 'bool', label: 'Hide unsafe results', default: true, help: 'Leave out results the service marks as unsafe.' },
    { key: 'priority', type: 'number', label: 'Priority', default: 25, help: 'Lower numbers win ties (1 to 50).' },
  ] },
  { id: 'thepiratebay', kind: 'indexer', version: '1', apiVersion: 1, name: 'The Pirate Bay', delivery: 'declarative', settings: [
    { key: 'baseUrl', type: 'url', label: 'Site address', default: 'https://apibay.org/', help: 'Address of the site. Change it only when the site moves.' },
    { key: 'priority', type: 'number', label: 'Priority', default: 25, help: 'Lower numbers win ties (1 to 50).' },
  ] },
  { id: 'harbourmusic', kind: 'indexer', version: '2', apiVersion: 1, name: 'Harbour Music', delivery: 'declarative', settings: [
    { key: 'baseUrl', type: 'url', label: 'Site address', default: 'https://harbour-music.example/', help: 'Address of the site.' },
    { key: 'cookie', type: 'secret', label: 'Session cookie', required: true, help: 'The Cookie header your browser sends while signed in.' },
    { key: 'priority', type: 'number', label: 'Priority', default: 25, help: 'Lower numbers win ties (1 to 50).' },
  ] },
];

export const indexerList: IndexerRow[] = [
  { key: 'dock', name: 'Dock Feeds', plugin: 'torznab', pluginName: 'Torznab', delivery: 'builtin', source: 'config', enabled: true, running: true,
    priority: 20, settings: { url: 'https://dock.example.net/api/torznab', apiPath: '/api', priority: 20 }, secretsSet: ['apiKey'],
    health: { ref: { type: 'indexer', key: 'dock' }, status: 'ok', since, consecutiveFailures: 0 }, torznab: '/torznab/dock/api' },
  { key: 'knaben', name: 'Knaben', plugin: 'knaben', pluginName: 'Knaben', delivery: 'declarative', source: 'ui', enabled: true, running: true,
    priority: 25, settings: { hideUnsafe: true }, secretsSet: [],
    health: { ref: { type: 'indexer', key: 'knaben' }, status: 'ok', since, consecutiveFailures: 0 }, torznab: '/torznab/knaben/api' },
  { key: 'harbour', name: 'Harbour Music', plugin: 'harbourmusic', pluginName: 'Harbour Music', delivery: 'declarative', source: 'ui', enabled: true,
    running: true, priority: 30, settings: {}, secretsSet: ['cookie'],
    health: { ref: { type: 'indexer', key: 'harbour' }, status: 'action_needed', since, consecutiveFailures: 1, lastErrorClass: 'session_expired',
      lastErrorMessage: 'Harbour Music no longer accepts the session cookie', fixHint: 'Sign in again from the integration settings.' },
    torznab: '/torznab/harbour/api' },
  { key: 'tpb', name: 'The Pirate Bay', plugin: 'thepiratebay', pluginName: 'The Pirate Bay', delivery: 'declarative', source: 'ui', enabled: false,
    running: false, priority: 40, settings: {}, secretsSet: [], torznab: '/torznab/tpb/api' },
];

export const definitionCatalogue: DefinitionEntry[] = [
  { id: 'harbourmusic', name: 'Harbour Music', origin: 'user', file: 'harbour-music.yaml', valid: true, privacy: 'semi-private', language: 'en',
    login: 'cookie', modes: ['search', 'music'], description: 'Music tracker behind a session cookie.' },
  { id: 'knaben', name: 'Knaben', origin: 'bundled', file: 'knaben.yaml', valid: true, privacy: 'public', language: 'en', login: 'none',
    modes: ['search', 'tv', 'movie', 'music', 'book'], description: 'Public meta-search across many torrent trackers, answered from a JSON API.' },
  { id: 'liveshows', name: 'liveshows', origin: 'user', file: 'liveshows.yaml', valid: false,
    problems: [{ path: 'search.response.rows', line: 31, message: 'an HTML response needs a row selector' }, { path: 'caps.categories[2].standard', line: 18, message: 'standard category 120 is outside 1000-9999' }] },
  { id: 'thepiratebay', name: 'The Pirate Bay', origin: 'bundled', file: 'thepiratebay.yaml', valid: true, privacy: 'public', language: 'en', login: 'none',
    modes: ['search', 'tv', 'movie', 'music', 'book'], description: 'Large public tracker, searched through its JSON API.' },
];

export const passedTest: IndexerTest = {
  ok: true, message: 'connected', elapsedMs: 640,
  caps: { modes: ['search', 'tv', 'movie', 'music', 'book'], idParams: ['imdbid'], categories: { 2000: 'Movies', 3000: 'Audio', 5000: 'TV' }, limit: 500 },
};

export const failedTest: IndexerTest = { ok: false, class: 'session_expired', message: 'Harbour Music no longer accepts the session cookie', elapsedMs: 220 };
