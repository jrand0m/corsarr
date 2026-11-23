import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import IconButton from '../IconButton/IconButton.svelte';

describe('Tooltip (via IconButton)', () => {
  it('opens on focus, closes on Escape, and does not double-announce the name', async () => {
    vi.useFakeTimers();
    const { getByRole } = render(IconButton, { icon: 'pause', label: 'Pause' });
    const btn = getByRole('button', { name: 'Pause' });
    const tip = getByRole('tooltip', { hidden: true });
    expect(btn.hasAttribute('aria-describedby')).toBe(false);
    expect(tip.getAttribute('aria-hidden')).toBe('true');
    await fireEvent.focus(btn);
    await vi.runAllTimersAsync();
    expect(tip.classList.contains('open')).toBe(true);
    await fireEvent.keyDown(btn, { key: 'Escape' });
    expect(tip.classList.contains('open')).toBe(false);
    vi.useRealTimers();
  });
});
