import { describe, expect, it } from 'vitest';
import { buildProperties } from './build';

const byKey = (b: Parameters<typeof buildProperties>[0]) => Object.fromEntries(buildProperties(b).map((p) => [p.key, p]));

describe('buildProperties', () => {
  it('shows a release build with a link to its tag', () => {
    const p = byKey({ version: 'v1.2.3', tag: 'v1.2.3', commit: 'abc123', dirty: false, buildDate: '2026-11-26T10:00:00Z', goVersion: 'go1.27.4' });
    expect(p.tag.href).toBe('https://github.com/jrand0m/corsarr/releases/tag/v1.2.3');
    expect(p.commit).toMatchObject({ value: 'abc123', copy: true });
    expect(p.tree.value).toBe('Clean');
    expect(p.goVersion.value).toBe('go1.27.4');
  });

  it('flags uncommitted changes and leaves unknown fields empty', () => {
    expect(byKey({ version: 'dev', commit: 'abc', dirty: true }).tree.value).toBe('Uncommitted changes');
    const p = byKey({ version: 'dev' });
    expect(p.tree.value).toBeUndefined();
    expect(p.tag.href).toBeUndefined();
  });
});
