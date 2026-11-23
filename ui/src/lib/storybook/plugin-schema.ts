// Synthetic plugin settings schemas for SchemaForm stories and tests (a fake plugin with
// every schema field type gets a working form with zero UI code).
import type { FieldMeta, FormValues, SettingsSchema } from '../forms/schema';

export const everyFieldSchema: SettingsSchema = {
  groups: [
    { id: 'connection', label: 'Connection', help: 'Where corsarr reaches the download client.' },
    { id: 'behaviour', label: 'Behaviour' },
    { id: 'advanced', label: 'Advanced' },
  ],
  fields: [
    { key: 'name', type: 'text', label: 'Name', required: true, default: 'Harbour box' },
    { key: 'url', type: 'url', label: 'Address', group: 'connection', required: true, placeholder: 'http://10.0.0.20:8080' },
    { key: 'username', type: 'text', label: 'Username', group: 'connection' },
    { key: 'password', type: 'password', label: 'Password', group: 'connection', secret: true },
    { key: 'apiKey', type: 'text', label: 'API key', group: 'connection', secret: true, required: true, help: 'Paste the key, or a reference like ${env:BOX_API_KEY}.' },
    { key: 'category', type: 'select', label: 'Category', group: 'behaviour', dynamicOptions: true, required: true, help: 'Read from the client when the form opens.' },
    { key: 'priority', type: 'select', label: 'Priority', group: 'behaviour', default: 'normal', options: [
      { value: 'low', label: 'Low' }, { value: 'normal', label: 'Normal' }, { value: 'high', label: 'High' },
    ] },
    { key: 'startPaused', type: 'bool', label: 'Add downloads paused', group: 'behaviour', default: false },
    { key: 'pauseReason', type: 'text', label: 'Pause note', group: 'behaviour', visibleWhen: { field: 'startPaused', equals: true }, help: 'Shown on the client next to held downloads.' },
    { key: 'kinds', type: 'multiselect', label: 'Media kinds', group: 'behaviour', default: ['tv', 'movie'], options: [
      { value: 'tv', label: 'TV' }, { value: 'movie', label: 'Movies' }, { value: 'music', label: 'Music' }, { value: 'book', label: 'Books' },
    ] },
    { key: 'slots', type: 'number', label: 'Parallel downloads', group: 'behaviour', default: 3, validation: { min: 1, max: 20 } },
    { key: 'stallAfter', type: 'duration', label: 'Stalled after', group: 'behaviour', default: 1800, durationUnits: ['m', 'h'] },
    { key: 'minFree', type: 'size', label: 'Keep free on disk', group: 'behaviour', default: 50_000_000_000, sizeUnits: ['MB', 'GB', 'TB'] },
    { key: 'completedPath', type: 'path', label: 'Completed folder', group: 'advanced', placeholder: '/downloads/complete' },
    { key: 'ignore', type: 'regex', label: 'Ignore files matching', group: 'advanced', placeholder: '\\.(nfo|txt)$' },
    { key: 'trackers', type: 'list', label: 'Extra trackers', group: 'advanced', itemLabel: 'tracker', mono: true, validation: { maxItems: 5 } },
    { key: 'headers', type: 'map', label: 'Extra request headers', group: 'advanced', mono: true },
  ],
};

export const everyFieldValues: FormValues = {
  name: 'Harbour box',
  url: 'http://10.0.0.20:8080',
  username: 'corsarr',
  category: 'tv-sonar',
  priority: 'normal',
  startPaused: false,
  kinds: ['tv', 'movie'],
  slots: 3,
  stallAfter: 1800,
  minFree: 50_000_000_000,
  completedPath: '/downloads/complete',
  ignore: '\\.(nfo|txt)$',
  trackers: ['udp://tracker.example.net:6969/announce'],
  headers: { 'X-Box-Client': 'corsarr' },
};

export const everyFieldMeta: Record<string, FieldMeta> = {
  password: { isSet: true },
  apiKey: { ref: '${env:BOX_API_KEY}' },
};

export const categoryOptions = [
  { value: 'tv-sonar', label: 'tv' },
  { value: 'movies', label: 'movies' },
  { value: 'music', label: 'music' },
];

export const plexSchema: SettingsSchema = {
  setup: [
    { kind: 'pinPairing', label: 'Pair with Plex', help: 'corsarr shows a code; enter it at plex.tv/link while signed in.', fills: ['token'] },
  ],
  fields: [
    { key: 'url', type: 'url', label: 'Server address', required: true },
    { key: 'token', type: 'text', label: 'Token', secret: true, required: true, help: 'Filled by pairing, or paste one.' },
    { key: 'refreshLibraries', type: 'bool', label: 'Refresh libraries after import', default: true },
  ],
};

export const trackerSchema: SettingsSchema = {
  setup: [
    { kind: 'cookiePaste', label: 'Use a browser session', fills: ['cookie'] },
    { kind: 'loginForm', label: 'Sign in to the tracker', help: 'Trades your password for a session cookie once.', fills: ['cookie'], otp: true },
  ],
  fields: [
    { key: 'url', type: 'url', label: 'Site', required: true, default: 'https://tracker.example.org' },
    { key: 'cookie', type: 'text', label: 'Session cookie', secret: true, required: true },
  ],
};
