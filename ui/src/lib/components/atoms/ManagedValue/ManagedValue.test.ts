import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import ManagedValue, { managedFrom, managedText } from './ManagedValue.svelte';

describe('ManagedValue', () => {
  it('names the key and the active source', () => {
    expect(managedText({ key: 'mediaKinds.tv.mode', kind: 'file', file: '/c.yml' })).toBe('Managed by mediaKinds.tv.mode via config file /c.yml');
    expect(managedText({ key: 'k', kind: 'env', name: 'CORSARR_K' })).toBe('Managed by k via environment variable CORSARR_K');
    expect(managedText({ key: 'k', kind: 'args', name: '--k' })).toBe('Managed by k via command-line argument --k');
    expect(managedText({ kind: 'args' })).toBe('Managed via command-line arguments');
    expect(managedText({ kind: 'env' })).toBe('Managed via environment variables');
    expect(managedText({ kind: 'file' })).toBe('Managed via the config file');
    expect(managedText({ key: 'k' })).toBe('Managed by k via the config');
  });

  it('derives the marker from the server source when only the key is known', () => {
    expect(managedFrom('mediaKinds.tv.mode', { kind: 'env' })).toEqual({ key: 'mediaKinds.tv.mode', kind: 'env', name: 'CORSARR_MEDIAKINDS_TV_MODE' });
    expect(managedFrom('a.b', { kind: 'args' })).toEqual({ key: 'a.b', kind: 'args', name: '--a.b' });
    expect(managedFrom('a.b', { kind: 'file', path: '/c.yml' })).toEqual({ key: 'a.b', kind: 'file', file: '/c.yml' });
    expect(managedFrom('a.b')).toEqual({ key: 'a.b' });
  });

  it('shows the value and lock with the sentence as tooltip and for screen readers', () => {
    const { container, getByText } = render(ManagedValue, { value: 'Shadow', managed: { key: 'k', kind: 'env', name: 'CORSARR_K' } });
    expect(getByText('Shadow')).toBeTruthy();
    const root = container.querySelector('.mv')!;
    expect(root.getAttribute('title')).toBe('Managed by k via environment variable CORSARR_K');
    expect(root.querySelector('.visually-hidden')!.textContent).toContain('CORSARR_K');
  });
});
