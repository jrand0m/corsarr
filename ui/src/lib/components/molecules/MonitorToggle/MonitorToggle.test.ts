import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import MonitorToggle from './MonitorToggle.svelte';
import GlobalStatus from '../GlobalStatus/GlobalStatus.svelte';

describe('MonitorToggle', () => {
  it('toggles with a stable name and aria-pressed', async () => {
    const onchange = vi.fn();
    const { getByRole } = render(MonitorToggle, { item: 'S02E03', onchange });
    const b = getByRole('button', { name: 'Monitor S02E03' });
    expect(b.getAttribute('aria-pressed')).toBe('true');
    await fireEvent.click(b);
    expect(onchange).toHaveBeenLastCalledWith('unmonitored');
    expect(b.getAttribute('aria-pressed')).toBe('false');
  });

  it('cannot toggle a pinned item; unpins when allowed', async () => {
    const onchange = vi.fn();
    const onunpin = vi.fn();
    const { getByRole, unmount } = render(MonitorToggle, { item: 'S02E05', state: 'pinned', reason: 'extended cut', onchange });
    const b = getByRole('button', { name: 'S02E05 is pinned' });
    expect(b.getAttribute('aria-disabled')).toBe('true');
    await fireEvent.click(b);
    expect(onchange).not.toHaveBeenCalled();
    unmount();
    const r = render(MonitorToggle, { item: 'S02E05', state: 'pinned', onunpin });
    await fireEvent.click(r.getByRole('button', { name: 'Unpin S02E05' }));
    expect(onunpin).toHaveBeenCalledOnce();
  });
});

describe('GlobalStatus', () => {
  it('names each indicator and links it', () => {
    const { getByRole } = render(GlobalStatus, { unhealthy: 2, active: 4, blocked: 1, connection: 'live', shadowKinds: ['Music'], healthHref: '#h', queueHref: '#q' });
    expect(getByRole('link', { name: '2 integrations need attention' }).getAttribute('href')).toBe('#h');
    expect(getByRole('link', { name: '4 downloading, 1 blocked' }).getAttribute('href')).toBe('#q');
    expect(getByRole('link', { name: 'Shadow mode: Music' })).toBeTruthy();
    expect(getByRole('status').textContent).toBe('');
  });

  it('announces a lost connection', async () => {
    const props = { unhealthy: 0, active: 0, blocked: 0, connection: 'live' as const };
    const { getByRole, rerender } = render(GlobalStatus, props);
    await rerender({ ...props, connection: 'offline' });
    expect(getByRole('status').textContent).toBe('Live updates disconnected');
  });
});
