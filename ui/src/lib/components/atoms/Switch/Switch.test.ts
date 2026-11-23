import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import Switch from './Switch.svelte';

describe('Switch', () => {
  it('toggles aria-checked and reports the new state', async () => {
    const onchange = vi.fn();
    const { getByRole } = render(Switch, { label: 'RSS sync', onchange });
    const sw = getByRole('switch', { name: 'RSS sync' });
    expect(sw.getAttribute('aria-checked')).toBe('false');
    await fireEvent.click(sw);
    expect(sw.getAttribute('aria-checked')).toBe('true');
    expect(onchange).toHaveBeenCalledWith(true);
  });

  it('keeps the accessible name when the label is hidden', () => {
    const { getByRole } = render(Switch, { label: 'Enable indexer', hideLabel: true });
    expect(getByRole('switch', { name: 'Enable indexer' })).toBeTruthy();
  });
});
