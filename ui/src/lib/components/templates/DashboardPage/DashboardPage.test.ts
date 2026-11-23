import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import DashboardPage from './DashboardPage.svelte';
import { downloads, NOW } from '../../../storybook/fixtures';

const attention = [{ id: 'a1', title: 'The Long Haul S01', detail: 'Only a sample file was found', status: { family: 'download' as const, value: 'import-blocked' }, href: '#/activity/queue/box/q3' }];

describe('DashboardPage', () => {
  it('puts the attention list first, then tiles, downloads, health and history', () => {
    const { getByRole, getAllByRole } = render(DashboardPage, {
      attention, tiles: [{ label: 'Wanted', value: 12 }], downloads: downloads.slice(0, 2), totalSpeed: 12.4e6,
      health: { ok: 6, total: 7, problems: [{ id: 'p', name: 'Usenet Pier', status: 'action_needed', message: 'API key refused' }] }, now: NOW,
    });
    const headings = getAllByRole('heading').map((h) => h.textContent);
    expect(headings.slice(0, 2)).toEqual(['Dashboard', 'Needs attention']);
    expect(getByRole('link', { name: /The Long Haul S01/ }).getAttribute('href')).toBe('#/activity/queue/box/q3');
    expect(getByRole('heading', { name: 'Downloading' })).toBeTruthy();
    expect(getAllByRole('article').length).toBe(2);
  });

  it('says so calmly when nothing needs attention, and shows empty queue', () => {
    const { getByText } = render(DashboardPage, { healthySince: 'since 09:12', health: { ok: 7, total: 7, problems: [] } });
    expect(getByText('Nothing needs you right now; all OK since 09:12.')).toBeTruthy();
    expect(getByText('Nothing downloading')).toBeTruthy();
    expect(getByText('7 of 7 integrations OK')).toBeTruthy();
  });

  it('fails one region without blanking the others and retries it', async () => {
    const onretry = vi.fn();
    const { getByText, getByRole } = render(DashboardPage, { downloads: downloads.slice(0, 1), errors: { health: 'timeout' }, onretry, now: NOW });
    expect(getByText("Couldn't load health")).toBeTruthy();
    expect(getByRole('heading', { name: 'Downloading' })).toBeTruthy();
    await fireEvent.click(getByRole('button', { name: /Retry/ }));
    expect(onretry).toHaveBeenCalledWith('health');
  });
});
