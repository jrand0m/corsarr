import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import LogViewer from './LogViewer.svelte';
import { logLines } from '../../../storybook/log-lines';
import { formatLogLine, matchesLog } from '../../../format/log';

const lines = logLines(48);

describe('log formatting', () => {
  it('filters by level, module and text (fields included) and formats a file line', () => {
    const l = lines.find((x) => x.level === 'error')!;
    expect(matchesLog(l, { minLevel: 'warn', module: '', text: '' })).toBe(true);
    expect(matchesLog(lines.find((x) => x.level === 'debug')!, { minLevel: 'info', module: '', text: '' })).toBe(false);
    expect(matchesLog(l, { minLevel: 'trace', module: 'import', text: '' })).toBe(false);
    expect(matchesLog(l, { minLevel: 'trace', module: '', text: 'backoff=5m' })).toBe(true);
    expect(formatLogLine(l)).toMatch(/^2026-11-22T\d\d:\d\d:\d\d\.\d{3}Z ERROR \[indexer\] Request failed: 503 Service Unavailable indexer=drunkenslug attempt=2 backoff=5m$/);
  });
});

describe('LogViewer', () => {
  it('keeps the DOM bounded for 50,000 lines', () => {
    const { container, getByText } = render(LogViewer, { lines: logLines(50_000), minLevel: 'trace' });
    expect(container.querySelectorAll('.line').length).toBeLessThan(80);
    expect(getByText('50,000 lines')).toBeTruthy();
  });

  it('filters by level and text and counts what is shown', async () => {
    const { getByRole, getByText, container } = render(LogViewer, { lines });
    await fireEvent.change(getByRole('combobox', { name: 'Lowest level shown' }), { target: { value: 'error' } });
    const errors = lines.filter((l) => l.level === 'error').length;
    expect(getByText(`${errors} of 48 lines`)).toBeTruthy();
    await fireEvent.change(getByRole('combobox', { name: 'Lowest level shown' }), { target: { value: 'trace' } });
    await fireEvent.input(getByRole('searchbox', { name: 'Filter log text' }), { target: { value: 'grab' } });
    expect(container.querySelectorAll('mark').length).toBeGreaterThan(0);
  });

  it('pauses and follows, and downloads the shown lines', async () => {
    const ondownload = vi.fn();
    const { getByRole, getByText } = render(LogViewer, { lines, minLevel: 'error', ondownload });
    await fireEvent.click(getByRole('button', { name: 'Pause' }));
    expect(getByText('Paused')).toBeTruthy();
    await fireEvent.click(getByRole('button', { name: 'Follow' }));
    await fireEvent.click(getByRole('button', { name: 'Download shown lines' }));
    const [text, name] = ondownload.mock.lastCall!;
    expect(name).toBe('corsarr.log');
    expect(text.trim().split('\n').every((l: string) => l.includes(' ERROR '))).toBe(true);
  });

  it('counts lines that arrive while paused', async () => {
    const { getByRole, rerender } = render(LogViewer, { lines, minLevel: 'trace', following: false });
    await rerender({ lines: [...lines, ...logLines(3, 48)], minLevel: 'trace', following: false });
    expect(getByRole('button', { name: '3 new lines' })).toBeTruthy();
  });

  it('shows a line whole with its fields when clicked', async () => {
    const { getAllByRole, getByRole } = render(LogViewer, { lines: lines.slice(0, 3), minLevel: 'trace' });
    await fireEvent.click(getAllByRole('button', { pressed: false })[0]);
    expect(getByRole('region', { name: 'Selected line' }).textContent).toContain(lines[0].message);
  });
});
