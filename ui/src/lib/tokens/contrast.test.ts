// WCAG 2.2 AA guard for the token palette: every semantic foreground/background pair the
// components use is checked in both themes. Add a pair here when a component combines two
// colour tokens in a new way.
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

// Read from disk: vitest stubs CSS imports (including ?raw) to empty strings.
const css = readFileSync(resolve(import.meta.dirname, 'tokens.css'), 'utf8');

type Rgb = [number, number, number];
function oklchToLinearRgb(L: number, C: number, H: number): { rgb: Rgb; inGamut: boolean } {
  const h = (H * Math.PI) / 180, a = C * Math.cos(h), b = C * Math.sin(h);
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;
  const raw: Rgb = [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ];
  return { rgb: raw.map((v) => Math.min(1, Math.max(0, v))) as Rgb, inGamut: raw.every((v) => v > -0.002 && v < 1.002) };
}
const luminance = ([r, g, b]: Rgb) => 0.2126 * r + 0.7152 * g + 0.0722 * b;
const contrast = (x: Rgb, y: Rgb) => {
  const [hi, lo] = [luminance(x), luminance(y)].sort((p, q) => q - p);
  return (hi + 0.05) / (lo + 0.05);
};

const themes = { light: new Map<string, ReturnType<typeof oklchToLinearRgb>>(), dark: new Map<string, ReturnType<typeof oklchToLinearRgb>>() };
const num = String.raw`([\d.]+)`;
const re = new RegExp(String.raw`--([\w-]+):\s*light-dark\(oklch\(${num}\s+${num}\s+${num}\),\s*oklch\(${num}\s+${num}\s+${num}\)\)`, 'g');
for (const m of css.matchAll(re)) {
  themes.light.set(m[1], oklchToLinearRgb(+m[2], +m[3], +m[4]));
  themes.dark.set(m[1], oklchToLinearRgb(+m[5], +m[6], +m[7]));
}

/** [foreground, background, minimum ratio] */
const pairs: [string, string, number][] = [
  ["ink", "surface-ground", 4.5],
  ["ink-muted", "surface-ground", 4.5],
  ["ink-subtle", "surface-ground", 4.5],
  ["accent-ink", "surface-ground", 4.5],
  ["line-strong", "surface-ground", 3],
  ["focus-ring", "surface-ground", 3],
  ["ink", "surface-raised", 4.5],
  ["ink-muted", "surface-raised", 4.5],
  ["ink-subtle", "surface-raised", 4.5],
  ["accent-ink", "surface-raised", 4.5],
  ["line-strong", "surface-raised", 3],
  ["focus-ring", "surface-raised", 3],
  ["ink", "surface-sunken", 4.5],
  ["ink-muted", "surface-sunken", 4.5],
  ["ink-subtle", "surface-sunken", 4.5],
  ["accent-ink", "surface-sunken", 4.5],
  ["line-strong", "surface-sunken", 3],
  ["focus-ring", "surface-sunken", 3],
  ["ink", "surface-overlay", 4.5],
  ["ink-muted", "surface-overlay", 4.5],
  ["ink-subtle", "surface-overlay", 4.5],
  ["accent-ink", "surface-overlay", 4.5],
  ["line-strong", "surface-overlay", 3],
  ["focus-ring", "surface-overlay", 3],
  ["ink", "surface-hover", 4.5],
  ["ink-muted", "surface-hover", 4.5],
  ["ink-subtle", "surface-hover", 4.5],
  ["accent-ink", "surface-hover", 4.5],
  ["line-strong", "surface-hover", 3],
  ["focus-ring", "surface-hover", 3],
  ["ink", "surface-selected", 4.5],
  ["ink-muted", "surface-selected", 4.5],
  ["ink-subtle", "surface-selected", 4.5],
  ["accent-ink", "surface-selected", 4.5],
  ["line-strong", "surface-selected", 3],
  ["focus-ring", "surface-selected", 3],
  ["ink-on-accent", "accent", 4.5],
  ["ink-on-accent", "accent-hover", 4.5],
  ["ink-on-accent", "accent-pressed", 4.5],
  ["accent", "surface-raised", 3],
  ["ink-inverse", "surface-inverse", 4.5],
  ["shell-ink", "shell-bg", 4.5],
  ["shell-ink-muted", "shell-bg", 4.5],
  ["shell-ink", "shell-active", 4.5],
  ["shell-ink-muted", "shell-hover", 4.5],
  ["ink", "accent-tint", 4.5],
  ["accent-ink", "accent-tint", 4.5],
  ["tone-ok-ink", "tone-ok-tint", 4.5],
  ["tone-ok-ink", "surface-raised", 4.5],
  ["tone-ok-ink", "surface-ground", 4.5],
  ["tone-ok-solid", "surface-raised", 3],
  ["tone-ok-solid", "surface-ground", 3],
  ["tone-ok-solid", "surface-sunken", 3],
  ["tone-warn-ink", "tone-warn-tint", 4.5],
  ["tone-warn-ink", "surface-raised", 4.5],
  ["tone-warn-ink", "surface-ground", 4.5],
  ["tone-warn-solid", "surface-raised", 3],
  ["tone-warn-solid", "surface-ground", 3],
  ["tone-warn-solid", "surface-sunken", 3],
  ["tone-error-ink", "tone-error-tint", 4.5],
  ["tone-error-ink", "surface-raised", 4.5],
  ["tone-error-ink", "surface-ground", 4.5],
  ["tone-error-solid", "surface-raised", 3],
  ["tone-error-solid", "surface-ground", 3],
  ["tone-error-solid", "surface-sunken", 3],
  ["tone-info-ink", "tone-info-tint", 4.5],
  ["tone-info-ink", "surface-raised", 4.5],
  ["tone-info-ink", "surface-ground", 4.5],
  ["tone-info-solid", "surface-raised", 3],
  ["tone-info-solid", "surface-ground", 3],
  ["tone-info-solid", "surface-sunken", 3],
  ["tone-neutral-ink", "tone-neutral-tint", 4.5],
  ["tone-neutral-ink", "surface-raised", 4.5],
  ["tone-neutral-ink", "surface-ground", 4.5],
  ["tone-neutral-solid", "surface-raised", 3],
  ["tone-neutral-solid", "surface-ground", 3],
  ["tone-neutral-solid", "surface-sunken", 3],
  ["tone-managed-ink", "tone-managed-tint", 4.5],
  ["tone-managed-ink", "surface-raised", 4.5],
  ["tone-managed-ink", "surface-ground", 4.5],
  ["tone-managed-solid", "surface-raised", 3],
  ["tone-managed-solid", "surface-ground", 3],
  ["tone-managed-solid", "surface-sunken", 3],
  ["ink-on-accent", "kind-tv", 4.5],
  ["kind-tv", "surface-raised", 3],
  ["ink-on-kind", "kind-tv-fill", 4.5],
  ["ink-on-accent", "kind-movie", 4.5],
  ["kind-movie", "surface-raised", 3],
  ["ink-on-kind", "kind-movie-fill", 4.5],
  ["ink-on-accent", "kind-music", 4.5],
  ["kind-music", "surface-raised", 3],
  ["ink-on-kind", "kind-music-fill", 4.5],
  ["ink-on-accent", "kind-book", 4.5],
  ["kind-book", "surface-raised", 3],
  ["ink-on-kind", "kind-book-fill", 4.5],
  ["ink-on-danger", "danger", 4.5],
  ["ink-on-danger", "danger-hover", 4.5],
  ["danger", "surface-raised", 3]
];

describe.each(['light', 'dark'] as const)('%s theme tokens', (theme) => {
  const t = themes[theme];
  it('parses the palette', () => expect(t.size).toBeGreaterThan(50));
  it('keeps every colour inside sRGB', () => {
    expect([...t].filter(([, v]) => !v.inGamut).map(([k]) => k)).toEqual([]);
  });
  it.each(pairs)('%s on %s ≥ %s:1', (fg, bg, min) => {
    expect(t.has(fg) && t.has(bg), `${fg} / ${bg} defined`).toBe(true);
    expect(contrast(t.get(fg)!.rgb, t.get(bg)!.rgb)).toBeGreaterThanOrEqual(min);
  });
});
