import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import Toast from './Toast.svelte';

afterEach(() => vi.useRealTimers());

describe('Toast', () => {
  it('is announced as status, or alert for failures', () => {
    const { getByRole, unmount } = render(Toast, { message: 'Removed', duration: 0 });
    expect(getByRole('status').textContent).toContain('Removed');
    unmount();
    expect(render(Toast, { message: 'Failed', tone: 'error', duration: 0 }).getByRole('alert')).toBeTruthy();
  });

  it('stays at least 10 s with undo, then dismisses', () => {
    vi.useFakeTimers();
    const ondismiss = vi.fn();
    render(Toast, { message: 'Removed', onundo: () => {}, ondismiss, duration: 3000 });
    vi.advanceTimersByTime(9_900);
    expect(ondismiss).not.toHaveBeenCalled();
    vi.advanceTimersByTime(200);
    expect(ondismiss).toHaveBeenCalledOnce();
  });

  it('pauses while hovered', async () => {
    vi.useFakeTimers();
    const ondismiss = vi.fn();
    const { getByRole } = render(Toast, { message: 'Held', ondismiss, duration: 10_000 });
    vi.advanceTimersByTime(5_000);
    await fireEvent.pointerEnter(getByRole('status'));
    vi.advanceTimersByTime(20_000);
    expect(ondismiss).not.toHaveBeenCalled();
    await fireEvent.pointerLeave(getByRole('status'));
    vi.advanceTimersByTime(5_100);
    expect(ondismiss).toHaveBeenCalledOnce();
  });

  it('undo calls back and dismisses once', async () => {
    const onundo = vi.fn();
    const ondismiss = vi.fn();
    const { getByRole } = render(Toast, { message: 'Removed', onundo, ondismiss, duration: 0 });
    await fireEvent.click(getByRole('button', { name: 'Undo' }));
    expect(onundo).toHaveBeenCalledOnce();
    expect(ondismiss).toHaveBeenCalledOnce();
  });
});
