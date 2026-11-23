import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import WizardPage from './WizardPage.svelte';

const steps = [{ id: 'plugin', label: 'Choose plugin' }, { id: 'connect', label: 'Connection' }];
const children = createRawSnippet((s: () => { label: string }) => ({ render: () => `<p>Step body: ${s().label}</p>` }));
const aside = createRawSnippet(() => ({ render: () => '<p>Help text</p>' }));

describe('WizardPage', () => {
  it('renders title, back link, the current step and help, and moves on', async () => {
    const onstep = vi.fn();
    const { getByRole, getByText } = render(WizardPage, {
      title: 'Add indexer', back: { href: '#/settings/indexers', label: 'Indexers' }, steps, children, aside, onstep,
    });
    expect(getByRole('heading', { level: 1, name: 'Add indexer' })).toBeTruthy();
    expect(getByRole('link', { name: /Indexers/ }).getAttribute('href')).toBe('#/settings/indexers');
    expect(getByText('Step body: Choose plugin')).toBeTruthy();
    expect(getByRole('complementary', { name: 'Help' })).toBeTruthy();
    await fireEvent.click(getByRole('button', { name: 'Continue' }));
    await new Promise((r) => setTimeout(r));
    expect(onstep).toHaveBeenCalledWith('connect');
  });
});
