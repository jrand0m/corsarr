import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent, within } from '@testing-library/svelte';
import DecisionPanel, { overrideLevel, type Decision, type DecisionReason } from './DecisionPanel.svelte';

const now = Date.parse('2026-11-22T14:05:00Z');
const base: Decision = {
  release: { title: 'Dockside.2026.1080p.WEB-DL.DDP5.1.H.264-HARBOUR', indexer: 'Harbour Tracker', protocol: 'torrent', size: 6.2e9, publishedAt: now - 7200_000, seeders: 1240, quality: 'WEB-DL 1080p', runtimeSeconds: 6600 },
  verdict: 'accept', rank: 1, candidates: 14,
  score: { total: 1250, formats: [{ name: 'DD+ 5.1', score: 250 }, { name: 'Trusted group', score: 1000 }] },
  reasons: [],
};
const pref: DecisionReason = { rule: 'size.max', category: 'preference', message: '58.2 GB is over the 40 GB cap' };
const temp: DecisionReason = { rule: 'seeders.min', category: 'temporary', message: '0 seeders, below the minimum of 1' };
const perm: DecisionReason = { rule: 'blocklist', category: 'permanent', message: 'On the blocklist since 20 Nov', blocklist: true };
const safe: DecisionReason = { rule: 'payload.executable', category: 'safety', message: 'Archive contains setup.exe' };
const reject = (reasons: DecisionReason[]): Decision => ({ ...base, verdict: 'reject', rank: 9, reasons });

describe('overrideLevel', () => {
  it('follows the override rule', () => {
    expect(overrideLevel([pref, temp])).toBe('allowed');
    expect(overrideLevel([pref, perm])).toBe('confirm');
    expect(overrideLevel([{ ...pref, blocklist: true }])).toBe('confirm');
    expect(overrideLevel([pref, perm, safe])).toBe('never');
  });
});

describe('DecisionPanel', () => {
  it('shows the release, facts, verdict, rank and score breakdown', () => {
    const { getByRole, container } = render(DecisionPanel, { decision: base, now });
    const panel = getByRole('region', { name: base.release.title });
    expect(panel.textContent).toContain('Accepted');
    expect(panel.textContent).toContain('Ranked 1 of 14');
    expect(panel.textContent).toContain('1,240 seeders');
    expect(container.querySelector('.total')!.textContent).toBe('Score +1,250');
    expect(within(getByRole('list', { name: 'Custom formats' })).getAllByRole('listitem').map((li) => li.textContent?.replace(/\s+/g, ' ').trim())).toEqual(['DD+ 5.1 +250', 'Trusted group +1000']);
  });

  it('marks a direct-source release instead of showing seeders', () => {
    const direct: Decision = { ...base, release: { ...base.release, protocol: 'source', seeders: undefined, indexer: 'TrubadurCamp' } };
    const { getByRole } = render(DecisionPanel, { decision: direct, now });
    const text = getByRole('region', { name: base.release.title }).textContent ?? '';
    expect(text).toContain('Direct source');
    expect(text).not.toContain('seeders');
  });

  it('lists every reason grouped by category, safety first, with explanations', () => {
    const { container } = render(DecisionPanel, { decision: reject([pref, temp, perm, safe]), now });
    const groups = [...container.querySelectorAll('.group')];
    expect(groups.map((g) => g.querySelector('.badge')!.textContent!.trim())).toEqual(['Safety', 'Permanent', 'Temporary', 'Preference']);
    expect(groups[0].textContent).toContain('Never overridden');
    expect(container.querySelectorAll('.group li')).toHaveLength(4);
    expect(container.textContent).toContain('Why it was rejected (4)');
  });

  it('grabs an accepted release with no overrides', async () => {
    const ongrab = vi.fn();
    const { getByRole } = render(DecisionPanel, { decision: base, now, ongrab });
    await fireEvent.click(getByRole('button', { name: 'Grab' }));
    expect(ongrab).toHaveBeenCalledWith([]);
  });

  it('grabs anyway directly for preference and temporary reasons', async () => {
    const ongrab = vi.fn();
    const { getByRole } = render(DecisionPanel, { decision: reject([pref, temp]), now, ongrab });
    await fireEvent.click(getByRole('button', { name: 'Grab anyway' }));
    expect(ongrab).toHaveBeenCalledWith([pref, temp]);
  });

  it('confirms before overriding permanent or blocklist reasons', async () => {
    const ongrab = vi.fn();
    const { getByRole } = render(DecisionPanel, { decision: reject([pref, perm]), now, ongrab });
    await fireEvent.click(getByRole('button', { name: 'Grab anyway…' }));
    expect(ongrab).not.toHaveBeenCalled();
    const dlg = getByRole('alertdialog');
    expect(dlg.textContent).toContain('Overrides: On the blocklist since 20 Nov');
    await fireEvent.click(within(dlg).getByRole('button', { name: 'Grab anyway' }));
    expect(ongrab).toHaveBeenCalledWith([pref, perm]);
  });

  it('never offers an override for safety reasons', () => {
    const { queryByRole, container } = render(DecisionPanel, { decision: reject([pref, safe]), now, ongrab: () => {} });
    expect(queryByRole('button', { name: /Grab/ })).toBeNull();
    expect(container.textContent).toContain('A safety rule failed');
    expect(container.textContent).toContain('No override');
  });
});
