import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, waitFor } from '@testing-library/svelte';
import App from './App.svelte';

type Routes = Record<string, unknown>;

function serve(routes: Routes) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const path = String(input).replace(/^api\/v1\//, '').split('?')[0];
    if (path in routes) {
      const body = routes[path];
      return typeof body === 'string' ? new Response(body, { status: 200 }) : new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } });
    }
    return new Response(JSON.stringify({ error: { code: 'not_found', message: 'no such endpoint' } }), { status: 404 });
  });
}

class StubEventSource {
  readyState = 0;
  onopen = null;
  onerror = null;
  addEventListener() {}
  close() {}
}

beforeEach(() => {
  vi.stubGlobal('EventSource', StubEventSource);
  location.hash = '';
});
afterEach(() => vi.unstubAllGlobals());

describe('App', () => {
  it('shows first run until the admin password is set', async () => {
    vi.stubGlobal('fetch', serve({ firstrun: { required: true, authMode: 'password' } }));
    const { findByRole } = render(App);
    expect((await findByRole('heading', { level: 1 })).textContent).toBe('Set up corsarr');
    expect(location.hash).toBe('#/setup');
  });

  it('sends a signed-out visitor to sign-in', async () => {
    vi.stubGlobal('fetch', serve({ firstrun: { required: false }, 'auth/me': { authenticated: false } }));
    const { findByRole } = render(App);
    expect((await findByRole('heading', { level: 1 })).textContent).toBe('Sign in to corsarr');
    expect(location.hash).toBe('#/login');
  });

  it('renders the shell and the dashboard when signed in', async () => {
    vi.stubGlobal('fetch', serve({
      firstrun: { required: false }, 'auth/me': { authenticated: true, method: 'session' },
      'system/status': { version: 'dev', enabledKinds: ['movie'], configSource: { kind: 'args' } },
      qualityprofiles: { items: [] }, 'health/integrations': { items: [] }, queue: { items: null }, movies: { items: [] },
      history: { items: [] }, 'wanted/missing': { items: [] }, 'config/export': 'CORSARR_MEDIAKINDS_MOVIE_MODE=active\n',
    }));
    const { findByRole, getByRole } = render(App);
    await waitFor(() => expect(getByRole('navigation', { name: 'Main' })).toBeTruthy());
    expect((await findByRole('heading', { level: 1, name: 'Dashboard' }, { timeout: process.env.CI ? 20000 : 3000 })).textContent).toBe('Dashboard');
  });

  it('shows a calm error when the server does not answer', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => { throw new TypeError('Failed to fetch'); }));
    const { findByText } = render(App);
    expect(await findByText("Couldn't reach corsarr")).toBeTruthy();
  });
});
