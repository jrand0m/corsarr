import { describe, it, expect } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import ItemRefSummary from './ItemRefSummary.svelte';

const items = [
  { label: 'S02E01', status: 'imported' as const },
  { label: 'S02E02', status: 'imported' as const },
  { label: 'S02E03', status: 'blocked' as const, reason: 'sample only' },
];

describe('ItemRefSummary', () => {
  it('counts link statuses, problems first, and expands to each link', async () => {
    const { getByRole, container } = render(ItemRefSummary, { summary: 'S02 · 3 episodes', items });
    expect(container.querySelector('.counts')!.textContent!.replace(/\s+/g, ' ').trim()).toBe('1 blocked 2 imported');
    const toggle = getByRole('button', { name: 'S02 · 3 episodes' });
    expect(toggle.getAttribute('aria-expanded')).toBe('false');
    expect(container.querySelector('ul')!.hidden).toBe(true);
    await fireEvent.click(toggle);
    expect(toggle.getAttribute('aria-expanded')).toBe('true');
    expect(container.querySelector('ul')!.hidden).toBe(false);
    expect(container.querySelector('ul')!.textContent).toContain('sample only');
  });

  it('says "all" when every link shares a status and does not expand a single item', () => {
    const one = render(ItemRefSummary, { summary: 'S02', items: items.slice(0, 2) });
    expect(one.container.textContent).toContain('Imported · all');
    one.unmount();
    const { queryByRole } = render(ItemRefSummary, { summary: 'Dockside', items: [items[2]] });
    expect(queryByRole('button')).toBeNull();
  });
});
