import { afterEach, describe, expect, it, vi } from 'vitest';
import { effectiveTheme } from './theme';

const prefersDark = (dark: boolean) =>
  vi.stubGlobal('matchMedia', (q: string) => ({ matches: dark && q === '(prefers-color-scheme: dark)', media: q }) as MediaQueryList);

describe('effectiveTheme', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('shows a pinned theme as chosen', () => {
    prefersDark(true);
    expect(effectiveTheme('light')).toBe('light');
    prefersDark(false);
    expect(effectiveTheme('dark')).toBe('dark');
  });

  it('follows the OS colour scheme on system', () => {
    prefersDark(true);
    expect(effectiveTheme('system')).toBe('dark');
    prefersDark(false);
    expect(effectiveTheme('system')).toBe('light');
  });

  it('reads light when the browser cannot report a colour scheme', () => {
    vi.stubGlobal('matchMedia', undefined);
    expect(effectiveTheme('system')).toBe('light');
  });
});
