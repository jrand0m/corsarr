import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent, within } from '@testing-library/svelte';
import { createRawSnippet, tick } from 'svelte';
import Wizard, { type WizardStep } from './Wizard.svelte';

const steps: WizardStep[] = [
  { id: 'plugin', label: 'Choose plugin' },
  { id: 'connect', label: 'Connection', description: 'Where corsarr reaches the indexer.' },
  { id: 'caps', label: 'Categories', optional: true },
  { id: 'review', label: 'Review' },
];
const children = createRawSnippet((s: () => WizardStep) => ({ render: () => `<p>Body of ${s().id}</p>` }));
const flush = async () => { await tick(); await Promise.resolve(); await tick(); };

describe('Wizard', () => {
  it('shows every step with the current one marked and its body', () => {
    const { getByRole, getByText } = render(Wizard, { steps, children });
    const nav = getByRole('navigation', { name: 'Steps' });
    expect(within(nav).getAllByRole('listitem')).toHaveLength(4);
    expect(nav.querySelector('[aria-current="step"]')!.textContent).toContain('Choose plugin');
    expect(getByRole('heading', { level: 2, name: 'Choose plugin' })).toBeTruthy();
    expect(getByText('Body of plugin')).toBeTruthy();
    expect(getByText('Optional')).toBeTruthy();
    // Later steps cannot be jumped to yet.
    expect(within(nav).queryByRole('button')).toBeNull();
  });

  it('validates before moving on, lists problems and focuses them', async () => {
    const validate = vi.fn((s: string) => (s === 'plugin' ? ['Choose a plugin', 'Name is required'] : []));
    const { getByRole, getByText } = render(Wizard, { steps, children, validate });
    await fireEvent.click(getByRole('button', { name: 'Continue' }));
    await flush();
    const alert = getByRole('alert');
    expect(alert.textContent).toContain('Fix these 2 things to continue');
    expect(alert.textContent).toContain('Name is required');
    expect(document.activeElement).toBe(alert);
    expect(getByText('Body of plugin')).toBeTruthy();
  });

  it('advances, marks steps finished, focuses the new heading, and goes back', async () => {
    const onstep = vi.fn();
    const { getByRole, getByText } = render(Wizard, { steps, children, onstep });
    await fireEvent.click(getByRole('button', { name: 'Continue' }));
    await flush();
    expect(onstep).toHaveBeenLastCalledWith('connect');
    expect(getByText('Body of connect')).toBeTruthy();
    expect(document.activeElement).toBe(getByRole('heading', { level: 2, name: 'Connection' }));
    expect(getByText('Where corsarr reaches the indexer.')).toBeTruthy();
    const nav = getByRole('navigation');
    expect(within(nav).getByRole('button').textContent).toContain('(finished)');
    await fireEvent.click(getByRole('button', { name: 'Back' }));
    expect(getByText('Body of plugin')).toBeTruthy();
  });

  it('resumes at a saved step and finishes on the last', async () => {
    const onfinish = vi.fn();
    const { getByRole, getByText } = render(Wizard, {
      steps, children, onfinish, resumed: true, current: 'review', completed: ['plugin', 'connect'], finishLabel: 'Add indexer',
    });
    expect(getByText(/Picked up where you left off/)).toBeTruthy();
    // Optional step skipped: review is still reachable; earlier steps are buttons.
    expect(within(getByRole('navigation')).getAllByRole('button')).toHaveLength(3);
    await fireEvent.click(getByRole('button', { name: 'Add indexer' }));
    await flush();
    expect(onfinish).toHaveBeenCalledOnce();
  });

  it('offers Cancel only with a handler', async () => {
    const oncancel = vi.fn();
    const { getByRole } = render(Wizard, { steps, children, oncancel });
    await fireEvent.click(getByRole('button', { name: 'Cancel' }));
    expect(oncancel).toHaveBeenCalled();
  });
});
