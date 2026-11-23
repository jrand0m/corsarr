import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import CheckResults from './CheckResults.svelte';
import SourceProgressStrip from '../SourceProgressStrip/SourceProgressStrip.svelte';

describe('CheckResults', () => {
  const checks = [
    { id: 'a', name: 'Reach', status: 'pass' as const, ms: 38 },
    { id: 'b', name: 'Save path', status: 'fail' as const, detail: 'not mounted', ms: 1500 },
    { id: 'c', name: 'Version', status: 'warn' as const },
  ];

  it('sorts problems first and summarises', () => {
    const { container, getByRole } = render(CheckResults, { checks });
    expect([...container.querySelectorAll('.name')].map((n) => n.textContent)).toEqual(['Save path', 'Version', 'Reach']);
    expect(getByRole('status').textContent).toBe('1 of 3 passed, 1 failed, 1 with warnings');
    expect(container.textContent).toContain('1.5 s');
  });

  it('shows progress and a busy rerun while running', async () => {
    const onrerun = vi.fn();
    const { getByRole, rerender } = render(CheckResults, { checks: [{ id: 'a', name: 'Reach', status: 'pass' as const }], onrerun });
    await fireEvent.click(getByRole('button', { name: 'Rerun' }));
    expect(onrerun).toHaveBeenCalledOnce();
    await rerender({ checks: [{ id: 'a', name: 'Reach', status: 'running' as const }, { id: 'b', name: 'Auth', status: 'pending' as const }], onrerun });
    expect(getByRole('status').textContent).toBe('Checking: 0 of 2 done');
    expect(getByRole('button', { name: 'Checking' }).getAttribute('aria-busy')).toBe('true');
  });
});

describe('SourceProgressStrip', () => {
  it('counts answers and announces only when all have answered', async () => {
    const sources = [
      { id: 'a', name: 'NZBgeek', state: 'ok' as const, count: 38 },
      { id: 'b', name: 'OldDock', state: 'running' as const },
    ];
    const { container, getByRole, rerender } = render(SourceProgressStrip, { sources, label: 'Indexers' });
    expect(container.querySelector('.sum')!.textContent).toBe('1 of 2 answered · 38 results');
    expect(getByRole('status').textContent).toBe('');
    await rerender({ sources: [sources[0], { id: 'b', name: 'OldDock', state: 'failed' as const, reason: 'timed out' }], label: 'Indexers' });
    expect(getByRole('status').textContent).toBe('All 2 indexers answered: 38 results, 1 failed');
    expect(container.textContent).toContain('timed out');
  });

  it('uses the singular for one result', () => {
    const { container } = render(SourceProgressStrip, { sources: [{ id: 'a', name: 'Pier', state: 'ok' as const, count: 1 }] });
    expect(container.querySelector('.sum')!.textContent).toBe('1 of 1 answered · 1 result');
    expect(container.textContent).toContain('1 result');
    expect(container.textContent).not.toContain('1 results');
  });
});
