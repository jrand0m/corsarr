import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import Chip from './Chip.svelte';

describe('Chip', () => {
  it('removes a filter token by an accessible button', async () => {
    const onremove = vi.fn();
    const { getByRole } = render(Chip, { variant: 'filter', facet: 'Profile', label: 'HD', onremove });
    await fireEvent.click(getByRole('button', { name: 'Remove Profile: HD' }));
    expect(onremove).toHaveBeenCalledOnce();
  });

  it('is a toggle button when clickable', () => {
    const { getByRole } = render(Chip, { label: 'Missing', selected: true, onclick: () => {} });
    expect(getByRole('button', { name: 'Missing' }).getAttribute('aria-pressed')).toBe('true');
  });

  it('signs scores', () => {
    const { container } = render(Chip, { variant: 'score', label: 'Remux', score: -50 });
    expect(container.textContent).toContain('−50');
  });
});
