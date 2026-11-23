import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import StatTile from './StatTile.svelte';

describe('StatTile', () => {
  it('is one link when it has href', () => {
    const { getByRole } = render(StatTile, { label: 'Downloading', value: 3, href: '#queue', linkLabel: 'Open queue' });
    const a = getByRole('link');
    expect(a.textContent!.replace(/\s+/g, ' ').trim()).toBe('Downloading 3 Open queue');
  });

  it('judges a trend by the better direction', () => {
    const down = render(StatTile, { label: 'Wanted', value: 42, trend: { delta: -6, better: 'down', period: 'vs last week' } });
    expect(down.container.querySelector('.trend')!.classList.contains('good')).toBe(true);
    expect(down.container.querySelector('.trend')!.textContent!.trim()).toBe('−6 vs last week');
    down.unmount();
    const up = render(StatTile, { label: 'Failed', value: 4, trend: { delta: 3, better: 'down', period: 'vs yesterday' } });
    expect(up.container.querySelector('.trend')!.classList.contains('bad')).toBe(true);
  });

  it('shows an em dash for unknown and a busy skeleton while loading', () => {
    const { getByLabelText, unmount } = render(StatTile, { label: 'Answers', value: null });
    expect(getByLabelText('Not known yet').textContent).toBe('—');
    unmount();
    const { container } = render(StatTile, { label: 'Wanted', value: 1, href: '#w', loading: true });
    expect(container.querySelector('a')).toBeNull();
    expect(container.querySelector('[aria-busy="true"]')).toBeTruthy();
  });
});
