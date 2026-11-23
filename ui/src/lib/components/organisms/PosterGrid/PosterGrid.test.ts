import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import PosterGrid, { type PosterItem } from './PosterGrid.svelte';

const items = (n: number): PosterItem[] =>
  Array.from({ length: n }, (_, i) => ({ id: `m${i}`, title: `Quay Lights ${i + 1}`, kind: 'movie', year: 2020 + (i % 6) }));
const tiles = (c: HTMLElement) => c.querySelectorAll<HTMLElement>('[data-index]');

describe('PosterGrid', () => {
  it('renders only rows near the viewport, so only their art loads', () => {
    const { container } = render(PosterGrid, { items: items(5000), label: 'Library' });
    expect(tiles(container).length).toBeGreaterThan(6);
    expect(tiles(container).length).toBeLessThan(80);
    expect(container.querySelectorAll('li[aria-setsize="5000"]').length).toBe(tiles(container).length);
  });

  it('moves between tiles with the arrow keys and opens with a click', async () => {
    const onopen = vi.fn();
    const list = items(30);
    const { container } = render(PosterGrid, { items: list, label: 'Library', onopen });
    const first = tiles(container)[0];
    expect(first.tabIndex).toBe(0);
    first.focus();
    await fireEvent.keyDown(first, { key: 'ArrowRight' });
    await new Promise((r) => setTimeout(r));
    expect((document.activeElement as HTMLElement).dataset.index).toBe('1');
    // 960 px fallback width with 132 px tiles → 6 columns.
    await fireEvent.keyDown(document.activeElement!, { key: 'ArrowDown' });
    await new Promise((r) => setTimeout(r));
    expect((document.activeElement as HTMLElement).dataset.index).toBe('7');
    await fireEvent.click(document.activeElement!);
    expect(onopen).toHaveBeenCalledWith(list[7]);
  });

  it('asks for the next page near the end and shows the empty state', () => {
    const onloadmore = vi.fn();
    render(PosterGrid, { items: items(10), label: 'Library', hasMore: true, onloadmore });
    expect(onloadmore).toHaveBeenCalledTimes(1);
    const { getByText } = render(PosterGrid, { items: [], label: 'Empty' });
    expect(getByText('Nothing to show')).toBeTruthy();
  });
});
