import { describe, it, expect } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import Timestamp from './Timestamp.svelte';

describe('Timestamp', () => {
  const now = new Date('2026-11-22T14:05:00Z');

  it('shows relative time with a machine-readable datetime', () => {
    const { container } = render(Timestamp, { value: new Date(now.getTime() - 300_000), now });
    const time = container.querySelector('time')!;
    expect(time.textContent).toBe('5m ago');
    expect(time.getAttribute('datetime')).toBe('2026-11-22T14:00:00.000Z');
  });

  it('is focusable and describes itself with the absolute time', async () => {
    const { container, getByRole } = render(Timestamp, { value: now, now });
    const time = container.querySelector('time')!;
    expect(time.getAttribute('tabindex')).toBe('0');
    await fireEvent.focus(time);
    const tip = getByRole('tooltip', { hidden: true });
    expect(time.getAttribute('aria-describedby')).toBe(tip.id);
    expect(tip.textContent).toMatch(/2026/);
  });

  it('can render absolute only, or drop out of the tab order', () => {
    const a = render(Timestamp, { value: now, mode: 'absolute' });
    expect(a.container.querySelector('time')!.textContent).toMatch(/22 Nov 2026/);
    a.unmount();
    const b = render(Timestamp, { value: now, now, focusable: false });
    expect(b.container.querySelector('time')!.hasAttribute('tabindex')).toBe(false);
  });
});
