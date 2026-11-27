// Build provenance as label/value rows for the About screen; same fields as `corsarr version --json`.
import type { Property } from '../lib/components/molecules/PropertyList/PropertyList.svelte';
import type { BuildInfo } from './api/types';

const REPO = 'https://github.com/jrand0m/corsarr';

export function buildProperties(b: Partial<BuildInfo>): Property[] {
  const tree = b.commit ? (b.dirty ? 'Uncommitted changes' : 'Clean') : undefined;
  return [
    { key: 'version', label: 'Version', value: b.version, mono: true },
    { key: 'tag', label: 'Release tag', value: b.tag, href: b.tag ? `${REPO}/releases/tag/${encodeURIComponent(b.tag)}` : undefined },
    { key: 'commit', label: 'Source commit', value: b.commit, copy: true },
    { key: 'tree', label: 'Source tree', value: tree },
    { key: 'buildDate', label: 'Build date', value: b.buildDate, mono: true },
    { key: 'goVersion', label: 'Go toolchain', value: b.goVersion, mono: true },
  ];
}
