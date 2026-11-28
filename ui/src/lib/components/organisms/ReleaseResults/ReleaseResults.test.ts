import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import ReleaseResults from './ReleaseResults.svelte';
import type { Decision } from '../DecisionPanel/DecisionPanel.svelte';

const decision = (title: string): Decision => ({
  release: { title, indexer: 'ix', protocol: 'torrent', size: 1e9, publishedAt: 0, seeders: 3, quality: 'WEB-DL 1080p' },
  verdict: 'accept', score: { total: 0, formats: [] }, reasons: [],
});
const options = [{ value: 'automation', label: 'Automation order' }, { value: 'size', label: 'Largest first' }];

describe('ReleaseResults', () => {
  it('lists the releases in the given order and reports a sort change', async () => {
    const onsort = vi.fn();
    const { getByLabelText, getByRole } = render(ReleaseResults, { items: [{ key: 'a', decision: decision('A') }, { key: 'b', decision: decision('B') }], sort: 'size', sortOptions: options, onsort });
    const list = getByRole('list', { name: 'Releases, largest first' });
    expect([...list.querySelectorAll('h3')].map((h) => h.textContent)).toEqual(['A', 'B']);
    const select = getByLabelText('Sort by') as HTMLSelectElement;
    expect(select.value).toBe('size');
    select.value = 'automation';
    await fireEvent.change(select);
    expect(onsort).toHaveBeenCalledWith('automation');
  });

  it('grabs by key and shows a note', async () => {
    const ongrab = vi.fn();
    const { getByRole, getByText } = render(ReleaseResults, { items: [{ key: 'a', decision: decision('A') }], sort: 'automation', sortOptions: options, ongrab, note: 'Nothing passed' });
    expect(getByText('Nothing passed').getAttribute('role')).toBe('status');
    await fireEvent.click(getByRole('button', { name: 'Grab' }));
    expect(ongrab).toHaveBeenCalledWith('a', []);
  });
});
