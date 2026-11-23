import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent, within } from '@testing-library/svelte';
import EvidencePanel, { type Evidence } from './EvidencePanel.svelte';

const now = Date.parse('2026-11-22T14:05:00Z');
const ev: Evidence = {
  rule: { id: 'stalled', name: 'Stalled downloads', href: '#rules/stalled' },
  mode: 'live',
  classification: 'would-act',
  verdict: 'No progress for 2h 14m with 0 peers, over the 2h limit.',
  thresholds: [
    { label: 'No progress for', limit: '2h', observed: '2h 14m', exceeded: true },
    { label: 'Peers', limit: '1', observed: '0', exceeded: true },
    { label: 'Age', limit: '30m', observed: '4h', exceeded: false },
  ],
  observations: [
    { at: now - 8040_000, text: 'Last progress at 81%' },
    { at: now - 600_000, text: 'Tracker reported 0 peers' },
  ],
  action: { summary: 'Removes the download, blocklists the release and searches again', verb: 'Removes', at: now + 245_000 },
  reasons: [
    { category: 'temporary', message: 'Zero peers right now' },
    { category: 'safety', message: 'Sample-only payload' },
  ],
  links: [{ label: 'Open download', href: '#queue/d2' }],
};

describe('EvidencePanel', () => {
  it('names the rule, mode, verdict and the action countdown', () => {
    const { getByRole, container } = render(EvidencePanel, { evidence: ev, now });
    const panel = getByRole('region', { name: 'Stalled downloads' });
    expect(getByRole('link', { name: 'Stalled downloads' }).getAttribute('href')).toBe('#rules/stalled');
    expect(panel.textContent).toContain('Live');
    expect(panel.textContent).toContain('Would act');
    expect(panel.textContent).toContain('over the 2h limit');
    expect(container.querySelector('.action time')!.textContent).toBe('in 4:05');
  });

  it('compares thresholds with observations in words, not colour alone', () => {
    const { getByRole } = render(EvidencePanel, { evidence: ev, now });
    const rows = within(getByRole('table')).getAllByRole('row').slice(1);
    expect(rows.map((r) => r.textContent)).toEqual([
      'No progress for2h2h 14mOver limit', 'Peers10Over limit', 'Age30m4hWithin limit',
    ]);
  });

  it('lists timestamped observations and groups reasons, safety first', () => {
    const { getByRole, container } = render(EvidencePanel, { evidence: ev, now });
    const obs = within(getByRole('region', { name: 'Observations' })).getAllByRole('listitem');
    expect(obs[1].querySelector('time')!.getAttribute('datetime')).toBe(new Date(now - 600_000).toISOString());
    expect(obs[1].textContent).toContain('Tracker reported 0 peers');
    const groups = [...container.querySelectorAll('.group')].map((g) => g.textContent);
    expect(groups[0]).toContain('Safety');
    expect(groups[1]).toContain('Temporary');
  });

  it('marks dry runs as "would"', () => {
    const { container } = render(EvidencePanel, { evidence: { ...ev, mode: 'dry-run' }, now });
    expect(container.textContent).toContain('Dry run: nothing changes.');
    expect(container.querySelector('.action')!.textContent).toContain('Would: Removes the download');
  });

  it('offers Undo only while the undo window is open and a handler exists', async () => {
    const onundo = vi.fn();
    const taken = { ...ev, classification: 'acted' as const, undoUntil: now + 86_400_000 };
    const r = render(EvidencePanel, { evidence: taken, now, onundo });
    await fireEvent.click(r.getByRole('button', { name: 'Undo' }));
    expect(onundo).toHaveBeenCalledOnce();
    expect(r.container.querySelector('.action time')).toBeNull();
    r.unmount();
    expect(render(EvidencePanel, { evidence: { ...taken, undoUntil: now - 1 }, now, onundo }).queryByRole('button', { name: 'Undo' })).toBeNull();
  });

  it('puts the panel behind a "Why?" popover', async () => {
    const { getByRole } = render(EvidencePanel, { evidence: ev, now, variant: 'popover' });
    const btn = getByRole('button', { name: 'Why? Stalled downloads' });
    const pop = document.getElementById(btn.getAttribute('popovertarget')!)!;
    expect(pop.getAttribute('popover')).toBe('auto');
    pop.showPopover();
    await Promise.resolve();
    expect(btn.getAttribute('aria-expanded')).toBe('true');
    expect(within(pop).getByRole('table')).toBeTruthy();
  });
});
