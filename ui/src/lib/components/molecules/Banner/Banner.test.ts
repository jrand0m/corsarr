import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import Banner from './Banner.svelte';
import EmptyState from '../EmptyState/EmptyState.svelte';
import ErrorState from '../ErrorState/ErrorState.svelte';

const text = (t: string) => createRawSnippet(() => ({ render: () => `<span>${t}</span>` }));

describe('Banner', () => {
  it('fills title from the preset and is a status region only when live', () => {
    const { getByText, container, unmount } = render(Banner, { preset: 'disconnected', children: text('Reconnecting.') });
    expect(getByText('Live updates paused')).toBeTruthy();
    expect(container.querySelector('[role="status"]')).toBeNull();
    unmount();
    const live = render(Banner, { preset: 'disconnected', live: true });
    expect(live.getByRole('status').textContent).toContain('Live updates paused');
  });

  it('dismisses', async () => {
    const ondismiss = vi.fn();
    const { getByRole } = render(Banner, { preset: 'restart', ondismiss });
    await fireEvent.click(getByRole('button', { name: 'Dismiss' }));
    expect(ondismiss).toHaveBeenCalledOnce();
  });
});

describe('EmptyState and ErrorState', () => {
  it('EmptyState titles the cause at the given heading level', () => {
    const { getByRole } = render(EmptyState, { title: 'Nothing is downloading', level: 2 });
    expect(getByRole('heading', { level: 2, name: 'Nothing is downloading' })).toBeTruthy();
  });

  it('ErrorState shows the code, retries and links to health', async () => {
    const onretry = vi.fn();
    const { getByRole, getByText, rerender } = render(ErrorState, { title: "Couldn't load the queue", code: 'HTTP 503', onretry, healthHref: '#health' });
    expect(getByText('HTTP 503')).toBeTruthy();
    await fireEvent.click(getByRole('button', { name: 'Retry' }));
    expect(onretry).toHaveBeenCalledOnce();
    expect(getByRole('link', { name: 'Open health' }).getAttribute('href')).toBe('#health');
    await rerender({ title: "Couldn't load the queue", onretry, retrying: true });
    expect(getByRole('button', { name: 'Retrying' }).getAttribute('aria-busy')).toBe('true');
  });

  it('ErrorState is an alert only when announced', () => {
    const { queryByRole, unmount } = render(ErrorState, { title: 'Failed' });
    expect(queryByRole('alert')).toBeNull();
    unmount();
    expect(render(ErrorState, { title: 'Failed', announce: true }).getByRole('alert')).toBeTruthy();
  });
});
