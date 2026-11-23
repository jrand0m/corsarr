// Typed handles for the CSS tokens in tokens.css. Components take these names as props;
// the values live only in CSS.
export const tones = ['ok', 'warn', 'error', 'info', 'neutral', 'managed'] as const;
export type Tone = (typeof tones)[number];

export const mediaKinds = ['tv', 'movie', 'music', 'book'] as const;
export type MediaKind = (typeof mediaKinds)[number];

export type Size = 'sm' | 'md' | 'lg';
export type Density = 'comfortable' | 'compact';

/** Token names, for Foundations stories and tests. */
export const tokenGroups = {
  surface: ['surface-ground', 'surface-raised', 'surface-sunken', 'surface-overlay', 'surface-hover', 'surface-pressed', 'surface-selected', 'surface-inverse'],
  ink: ['ink', 'ink-muted', 'ink-subtle', 'ink-disabled', 'ink-inverse', 'ink-on-accent'],
  line: ['line', 'line-strong'],
  accent: ['accent', 'accent-hover', 'accent-pressed', 'accent-ink', 'accent-tint', 'focus-ring'],
  shell: ['shell-bg', 'shell-ink', 'shell-ink-muted', 'shell-hover', 'shell-active', 'shell-line'],
  kind: ['kind-tv', 'kind-movie', 'kind-music', 'kind-book'],
  text: ['text-2xs', 'text-xs', 'text-sm', 'text-md', 'text-base', 'text-lg', 'text-xl', 'text-2xl'],
  space: ['space-0-5', 'space-1', 'space-1-5', 'space-2', 'space-3', 'space-4', 'space-5', 'space-6', 'space-8', 'space-10', 'space-12', 'space-16'],
  radius: ['radius-xs', 'radius-sm', 'radius-md', 'radius-lg', 'radius-round'],
  shadow: ['shadow-sm', 'shadow-md', 'shadow-lg'],
  duration: ['dur-instant', 'dur-fast', 'dur-base', 'dur-slow'],
} as const;
