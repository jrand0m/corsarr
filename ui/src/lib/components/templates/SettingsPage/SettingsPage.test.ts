import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import SettingsPage from './SettingsPage.svelte';
import { torznabSchema, torznabValues, torznabMeta, torznabManagedMeta } from '../../../storybook/fixtures';

const sections = [
  { id: 'general', label: 'General', href: '#/settings/general' },
  { id: 'indexers', label: 'Indexers & sources', href: '#/settings/indexers', count: 4, attention: true },
];

describe('SettingsPage', () => {
  it('lists sections with the current one marked, and renders the schema form', async () => {
    const onsave = vi.fn();
    const test = createRawSnippet(() => ({ render: () => '<p>Test results</p>' }));
    const { getAllByRole, getByRole, getByText } = render(SettingsPage, {
      sections, current: 'indexers', title: 'Harbour Tracker', schema: torznabSchema, values: torznabValues, meta: torznabMeta, onsave, test,
    });
    const nav = getByRole('navigation', { name: 'Settings sections' });
    expect(nav.querySelector('[aria-current="page"]')!.textContent).toContain('Indexers & sources');
    expect(getAllByRole('heading', { level: 1 })[0].textContent).toBe('Harbour Tracker');
    expect(getByRole('form', { name: 'Harbour Tracker' })).toBeTruthy();
    expect(getByText('Test results')).toBeTruthy();
    await fireEvent.input(getByRole('textbox', { name: /^Name/ }), { target: { value: 'Harbour' } });
    await fireEvent.submit(getByRole('form', { name: 'Harbour Tracker' }));
    await new Promise((r) => setTimeout(r));
    expect(onsave).toHaveBeenCalled();
  });

  it('locks the form and names the source when managed by config', () => {
    const { getByText, getAllByText, getByRole } = render(SettingsPage, {
      sections, current: 'indexers', title: 'Dock Feeds', schema: torznabSchema, values: torznabValues, meta: torznabManagedMeta,
      managed: 'indexers.dock_feeds', managedSource: 'corsarr.yaml, line 42',
    });
    expect(getAllByText('Managed by config').length).toBeGreaterThan(0);
    expect(() => getByText(/Every setting here comes from the config file/)).toThrow();
    expect(getByText('indexers.dock_feeds')).toBeTruthy();
    expect((getByRole('textbox', { name: /^Name/ }) as HTMLInputElement).readOnly).toBe(true);
  });
});
