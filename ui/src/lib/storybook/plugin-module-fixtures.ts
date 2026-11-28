// Fixtures for the plugin module components and pages: a download-client module with every
// field type, groups and actions; installed modules; a catalogue with every offer state; and a
// hostile schema whose texts try to become markup.
import type { CatalogueEntry, CatalogueView, ModuleInfo, ModuleManifest, PluginPolicy } from '../../app/api/types';
import type { ModuleSchema } from '../forms/plugin-types';

export const harbourSchema: ModuleSchema = {
  fields: [
    { key: 'url', type: 'string', label: 'Address', pattern: 'url-http', required: true, description: 'Where the Harbour web UI listens, with the port.', hint: 'For example http://10.0.0.20:8080' },
    { key: 'username', type: 'string', label: 'Username', maxLength: 64 },
    { key: 'apiKey', type: 'secret', label: 'API key', required: true, description: 'Settings › Remote access in Harbour 🔑' },
    { key: 'category', type: 'string', label: 'Category', pattern: 'identifier', default: 'corsarr' },
    { key: 'paused', type: 'bool', label: 'Add paused', default: false, description: 'Queue downloads without starting them.' },
    { key: 'slots', type: 'int', label: 'Parallel downloads', min: 1, max: 20, default: 4 },
    { key: 'ratio', type: 'number', label: 'Seed ratio', min: 0, max: 10, decimals: 2 },
    { key: 'priority', type: 'enum', label: 'Priority', default: 'normal', options: [{ value: 'low', label: 'Low 🐢' }, { value: 'normal', label: 'Normal' }, { value: 'high', label: 'High ⚡' }] },
    { key: 'timeout', type: 'duration', label: 'Request timeout', minDuration: '5s', maxDuration: '5m', default: '30s' },
    { key: 'trackers', type: 'list<string>', label: 'Extra trackers', maxItems: 10 },
    { key: 'ports', type: 'list<int>', label: 'Listen ports', maxItems: 3 },
    { key: 'paths', type: 'map<string,string>', label: 'Path mappings', maxEntries: 10, description: 'Harbour folder → corsarr folder 📁' },
  ],
  groups: [
    { title: 'Connection', description: 'How corsarr reaches Harbour.', fields: ['url', 'username', 'apiKey'] },
    { title: 'Downloads', fields: ['category', 'paused', 'slots', 'ratio', 'priority', 'timeout', 'trackers', 'ports', 'paths'] },
  ],
  actions: [{ id: 'test', label: 'Test connection' }, { id: 'discover', label: 'Find categories' }],
};

export const harbourManifest: ModuleManifest = {
  id: 'harbourdl', name: 'Harbour', version: '1.3.0', kind: 'downloader', abiVersion: 1,
  description: 'Sends downloads to a Harbour client and follows their progress.\nWorks with Harbour 2.0 and later ✅',
  author: 'Harbour contributors', license: 'MIT', homepage: 'https://harbour.example/corsarr',
  permissions: { http: { hosts: ['$setting:url'] }, secrets: ['apiKey'] },
  limits: { memoryMiB: 32, callTimeoutMs: 5000, concurrency: 2 },
  settings: harbourSchema,
};

export const policyOpen: PluginPolicy = { uiInstall: true, autoUpdate: 'patch', catalogueUrl: 'https://plugins.example/index.json', lockKey: 'plugins.uiInstall' };
export const policyLocked: PluginPolicy = { ...policyOpen, uiInstall: false };

export const harbourModule: ModuleInfo = {
  id: 'harbourdl', version: '1.3.0', previous: '1.2.0', sha256: '9f2c1a7e4b0d55c3e8a1f6d2b7c4e9a0f3d8b2c6e1a5f9d4b8c3e7a2f6d1b5c9', kind: 'downloader',
  status: 'active', instances: 2, ops: ['add', 'list', 'remove', 'test'], manifest: harbourManifest,
  usedBy: [{ section: 'downloadClients', key: 'upstairs', name: 'Upstairs box', enabled: true }], source: 'catalogue',
};

export const modules: ModuleInfo[] = [
  harbourModule,
  {
    id: 'quaydl', version: '0.4.1', sha256: '1b5c9f2c1a7e4b0d55c3e8a1f6d2b7c4e9a0f3d8b2c6e1a5f9d4b8c3e7a2f6d1', kind: 'downloader', status: 'failed',
    error: 'the module asked for 512 MiB; the limit is 64 MiB', instances: 0,
    manifest: { ...harbourManifest, id: 'quaydl', name: 'Quay', version: '0.4.1', description: 'Quay download client.', homepage: undefined,
      permissions: { http: { hosts: ['$setting:url', 'api.quay.example'] } }, settings: { fields: [{ key: 'url', type: 'string', label: 'Address', pattern: 'url-http' }] } },
    usedBy: [], source: 'file',
  },
  {
    id: 'pierdl', version: '2.0.0', sha256: 'e7a2f6d1b5c99f2c1a7e4b0d55c3e8a1f6d2b7c4e9a0f3d8b2c6e1a5f9d4b8c3', kind: 'downloader', status: 'uninstalled', instances: 0,
    manifest: { ...harbourManifest, id: 'pierdl', name: 'Pier', version: '2.0.0', description: 'Pier download client.' },
    usedBy: [{ section: 'downloadClients', key: 'pier', name: 'Pier', enabled: true }], source: 'catalogue',
  },
];

/** The env export the instance pages read values from. */
export const harbourEnvExport = [
  'CORSARR_DOWNLOADCLIENTS_UPSTAIRS_TYPE=harbourdl',
  'CORSARR_DOWNLOADCLIENTS_UPSTAIRS_NAME=Upstairs box',
  'CORSARR_DOWNLOADCLIENTS_UPSTAIRS_URL=http://10.0.0.20:8080',
  'CORSARR_DOWNLOADCLIENTS_UPSTAIRS_USERNAME=media',
  'CORSARR_DOWNLOADCLIENTS_UPSTAIRS_APIKEY=',
  'CORSARR_DOWNLOADCLIENTS_UPSTAIRS_SLOTS=6',
  'CORSARR_DOWNLOADCLIENTS_UPSTAIRS_TIMEOUT=1m',
  'CORSARR_DOWNLOADCLIENTS_UPSTAIRS_TRACKERS_0=udp://tracker.example:1337',
  'CORSARR_DOWNLOADCLIENTS_UPSTAIRS_PATHS_DOWNLOADS=/data/downloads',
].join('\n');

const entry = (id: string, name: string, version: string, over: Partial<CatalogueEntry> = {}): CatalogueEntry => ({
  id, name, version, kind: 'downloader', abiVersion: 1, description: `${name} download client.`, author: `${name} contributors`, license: 'MIT',
  permissions: { http: { hosts: ['$setting:url'] } }, size: 412_000, sha256: 'ab'.repeat(32), url: `https://plugins.example/${id}-${version}.wasm`, ...over,
});

export const catalogue: CatalogueView = {
  source: { url: 'https://plugins.example/index.json', release: '2026.11.3', generated: '2026-11-27T08:00:00Z', fetchedAt: '2026-11-27T09:12:00Z', stale: false },
  policy: policyOpen,
  offers: [
    { state: 'available', entry: entry('dockdl', 'Dock', '1.0.2', { description: 'Dock download client with categories and tags ⚡', homepage: 'https://dock.example' }) },
    { state: 'update', installed: '1.2.0', entry: entry('harbourdl', 'Harbour', '1.3.0', { permissions: harbourManifest.permissions }), diff: { addedHosts: [], removedHosts: [], addedSecrets: [], removedSecrets: [] } },
    {
      state: 'update_needs_approval', installed: '0.4.1',
      entry: entry('quaydl', 'Quay', '0.5.0', { permissions: { http: { hosts: ['$setting:url', 'api.quay.example', 'metrics.quay.example'] }, secrets: ['token'] } }),
      diff: { addedHosts: ['metrics.quay.example'], removedHosts: [], addedSecrets: ['token'], removedSecrets: [] },
    },
    { state: 'installed', installed: '2.0.0', entry: entry('pierdl', 'Pier', '2.0.0') },
    { state: 'older', installed: '3.1.0', entry: entry('wharfdl', 'Wharf', '3.0.0') },
    { state: 'incompatible', reason: 'needs corsarr ≥ 1.5.0', entry: entry('jettydl', 'Jetty', '1.0.0', { minCorsarr: '1.5.0' }) },
  ],
};

export const catalogueStale: CatalogueView = {
  ...catalogue,
  source: { ...catalogue.source, fetchedAt: '2026-11-25T09:12:00Z', stale: true, error: 'plugins.example did not answer within 10 s' },
};

export const catalogueFailed: CatalogueView = {
  source: { url: 'https://plugins.example/index.json', stale: false, error: 'the index signature does not match the pinned key' },
  policy: policyOpen,
  offers: [],
};

/** Texts that try to become markup, links, styles or scripts. */
export const hostileTexts = [
  '<img src=x onerror=alert(1)>',
  '<script>alert("x")</script>',
  '"><svg onload=alert(1)>',
  '<a href="javascript:alert(1)">click</a>',
  '<style>body{display:none}</style>',
  '{@html "<b>x</b>"}',
  'javascript:alert(1)',
  'https://evil.example/steal',
  '‮evil‬ and ​zero-width',
  '<iframe src="https://evil.example"></iframe>',
  '&lt;b&gt;entity&lt;/b&gt;',
  '[link](https://evil.example) **bold**',
];

export const hostileSchema: ModuleSchema = {
  fields: [
    { key: 'url', type: 'string', label: hostileTexts[0], description: hostileTexts[1], pattern: 'url-http' },
    { key: 'mode', type: 'enum', label: hostileTexts[2], options: [{ value: 'a', label: hostileTexts[3] }, { value: 'b', label: hostileTexts[4] }] },
    { key: 'note', type: 'string', label: hostileTexts[5], description: hostileTexts[6] },
  ],
  groups: [{ title: hostileTexts[9], description: hostileTexts[7], fields: ['note'] }],
  actions: [{ id: 'test', label: hostileTexts[10] }],
};

/** A schema with a type outside the closed set. */
export const unsupportedSchema = { fields: [{ key: 'body', type: 'html', label: '<b>Body</b>' }] } as ModuleSchema;
