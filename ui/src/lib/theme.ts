// Theme preference: "system" follows the OS (color-scheme: light dark); "light"/"dark" pin it
// via <html data-theme>. Persisted per browser; storage failures fall back to system.
export type ThemePreference = 'system' | 'light' | 'dark';
const KEY = 'corsarr.theme';

export function readTheme(): ThemePreference {
  try {
    const v = localStorage.getItem(KEY);
    return v === 'light' || v === 'dark' ? v : 'system';
  } catch {
    return 'system';
  }
}

export function applyTheme(pref: ThemePreference, root: HTMLElement = document.documentElement): void {
  if (pref === 'system') delete root.dataset.theme;
  else root.dataset.theme = pref;
  try {
    if (pref === 'system') localStorage.removeItem(KEY);
    else localStorage.setItem(KEY, pref);
  } catch {
    /* private mode: preference lasts for this page only */
  }
}

/** The theme actually showing, for picking a toggle icon. */
export function effectiveTheme(pref: ThemePreference): 'light' | 'dark' {
  if (pref !== 'system') return pref;
  return typeof matchMedia === 'function' && matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}
