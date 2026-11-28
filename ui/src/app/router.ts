// The route table of the web UI over the hash helpers in lib/route/hash.ts. Pure: a
// path maps to an area (one lazily loaded chunk), a screen inside it and its path params.
// Routes the MVP does not ship yet resolve to `later`, so links from the nav never dead-end.

export type Area = 'access' | 'dashboard' | 'library' | 'wanted' | 'activity' | 'system' | 'settings';

export interface Match {
  area: Area;
  screen: string;
  params: Record<string, string>;
}

export type Resolved = Match | { redirect: string } | { later: string } | null;

const routes: [pattern: string, area: Area, screen: string][] = [
  ['/setup', 'access', 'setup'],
  ['/login', 'access', 'login'],
  ['/', 'dashboard', 'dashboard'],
  ['/library/movies', 'library', 'movies'],
  ['/library/movies/add', 'library', 'add'],
  ['/library/movies/:id', 'library', 'movie'],
  ['/library/tv', 'library', 'series'],
  ['/library/tv/add', 'library', 'addSeries'],
  ['/library/tv/:id', 'library', 'show'],
  ['/library/music', 'library', 'artists'],
  ['/library/music/add', 'library', 'addArtist'],
  ['/library/music/albums/:id', 'library', 'album'],
  ['/library/music/:id', 'library', 'artist'],
  ['/wanted/missing', 'wanted', 'missing'],
  ['/wanted/missing/tv', 'wanted', 'missingTv'],
  ['/wanted/missing/music', 'wanted', 'missingMusic'],
  ['/activity/queue', 'activity', 'queue'],
  ['/activity/history', 'activity', 'history'],
  ['/activity/blocklist', 'activity', 'blocklist'],
  ['/system/health', 'system', 'health'],
  ['/system/tasks', 'system', 'tasks'],
  ['/system/migration', 'system', 'migration'],
  ['/system/about', 'system', 'about'],
  ['/settings/general', 'settings', 'general'],
  ['/settings/plugins', 'settings', 'plugins'],
  ['/settings/plugins/:id', 'settings', 'plugins'],
  ['/settings/indexers', 'settings', 'indexers'],
  ['/settings/profiles', 'settings', 'profiles'],
  ['/settings/custom-formats', 'settings', 'custom-formats'],
];

const redirects: Record<string, string> = {
  '/library': '/library/movies',
  '/wanted': '/wanted/missing',
  '/activity': '/activity/queue',
  '/system': '/system/health',
  '/settings': '/settings/general',
};

/** Known web UI areas whose screens are not built yet (shown as "not in this version yet"). */
const laterPrefixes = ['/library/', '/wanted/', '/activity/', '/search', '/settings/', '/system/'];

function matchPattern(pattern: string, path: string): Record<string, string> | null {
  const a = pattern.split('/');
  const b = path.split('/');
  if (a.length !== b.length) return null;
  const params: Record<string, string> = {};
  for (let i = 0; i < a.length; i++) {
    if (a[i].startsWith(':')) {
      if (!b[i]) return null;
      params[a[i].slice(1)] = decodeURIComponent(b[i]);
    } else if (a[i] !== b[i]) return null;
  }
  return params;
}

export function resolve(rawPath: string): Resolved {
  const path = rawPath.length > 1 ? rawPath.replace(/\/+$/, '') : rawPath;
  if (redirects[path]) return { redirect: redirects[path] };
  for (const [pattern, area, screen] of routes) {
    const params = matchPattern(pattern, path);
    if (params) return { area, screen, params };
  }
  if (laterPrefixes.some((p) => path.startsWith(p))) return { later: path };
  return null;
}

/** Access screens render without the shell and without a session. */
export const isAccess = (path: string): boolean => path === '/setup' || path === '/login';
