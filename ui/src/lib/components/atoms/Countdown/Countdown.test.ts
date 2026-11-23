import { describe, it, expect, vi } from 'vitest';
import { render } from '@testing-library/svelte';
import Countdown from './Countdown.svelte';

describe('Countdown', () => {
  const now = Date.parse('2026-11-22T14:00:00Z');

  it('shows a clock inside the last hour with a stable spoken text', () => {
    const { container } = render(Countdown, { to: now + 245_000, now, action: 'Removes' });
    expect(container.querySelector('time')!.textContent).toBe('in 4:05');
    expect(container.querySelector('.visually-hidden')!.textContent).toBe('Removes in 5m');
  });

  it('shows compact durations beyond an hour', () => {
    const { container } = render(Countdown, { to: now + 3 * 3600_000 + 20 * 60_000, now });
    expect(container.querySelector('time')!.textContent).toBe('in 3h 20m');
  });

  it('fires onexpire once at zero and shows the expired label', async () => {
    const onexpire = vi.fn();
    const { container, rerender } = render(Countdown, { to: now + 1000, now, onexpire, expiredLabel: 'acting now' });
    expect(onexpire).not.toHaveBeenCalled();
    await rerender({ to: now + 1000, now: now + 2000, onexpire, expiredLabel: 'acting now' });
    await rerender({ to: now + 1000, now: now + 3000, onexpire, expiredLabel: 'acting now' });
    expect(onexpire).toHaveBeenCalledOnce();
    expect(container.querySelector('time')!.textContent).toBe('acting now');
  });

  it('does not tick by the second under reduced motion', () => {
    const mm = window.matchMedia;
    window.matchMedia = ((q: string) => ({ ...mm(q), matches: q.includes('reduce') })) as typeof window.matchMedia;
    const { container } = render(Countdown, { to: now + 245_000, now });
    expect(container.querySelector('time')!.textContent).toBe('in 5m');
    window.matchMedia = mm;
  });
});
