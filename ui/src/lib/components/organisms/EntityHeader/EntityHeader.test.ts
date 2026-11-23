import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import EntityHeader from './EntityHeader.svelte';

const actions = createRawSnippet(() => ({ render: () => '<div><button>Search monitored</button></div>' }));

describe('EntityHeader', () => {
  it('renders the title as the page heading with kind, meta, states and facts', () => {
    const { getByRole, container } = render(EntityHeader, {
      title: 'The Long Haul', kind: 'tv', meta: '2024 · Harbourline', art: null,
      status: [{ family: 'item', value: 'cutoff-unmet', detail: '3 episodes' }],
      facts: [{ label: 'On disk', value: '38 of 41' }, { label: 'Size', value: '182 GB' }],
    });
    expect(getByRole('heading', { level: 1, name: 'The Long Haul' })).toBeTruthy();
    expect(container.querySelector('.meta')!.textContent).toContain('TV series · 2024 · Harbourline');
    expect(container.textContent).toContain('Below cutoff · 3 episodes');
    expect([...container.querySelectorAll('dt')].map((d) => d.textContent)).toEqual(['On disk', 'Size']);
    // Missing art falls back to initials labelled with the title.
    expect(getByRole('img', { name: 'The Long Haul' })).toBeTruthy();
  });

  it('renders actions, the overflow menu and the monitor toggle', async () => {
    const onmenu = vi.fn();
    const onmonitor = vi.fn();
    const { getByRole, getByText } = render(EntityHeader, {
      title: 'Dockside', kind: 'movie', actions, monitor: 'monitored', onmonitor,
      menu: [{ id: 'delete', label: 'Delete…', danger: true }], onmenu,
      back: { href: '#/library/movies', label: 'Movies' },
    });
    expect(getByRole('button', { name: 'Search monitored' })).toBeTruthy();
    expect(getByRole('link', { name: 'Movies' })).toBeTruthy();
    await fireEvent.click(getByText('Delete…'));
    expect(onmenu).toHaveBeenCalledWith('delete');
    expect(getByRole('button', { name: 'More actions for Dockside' })).toBeTruthy();
  });

  it('keeps RTL and CJK titles direction-aware', () => {
    const { getByRole } = render(EntityHeader, { title: 'مرفأ الضوء', kind: 'book' });
    expect(getByRole('heading', { level: 1 }).getAttribute('dir')).toBe('auto');
  });
});
