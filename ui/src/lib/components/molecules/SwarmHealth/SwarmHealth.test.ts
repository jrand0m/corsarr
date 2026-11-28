import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import SwarmHealth from './SwarmHealth.svelte';

describe('SwarmHealth', () => {
  it('shows seeders, leechers, a health word and a described ratio bar', () => {
    const { container, getByRole } = render(SwarmHealth, { seeders: 2, leechers: 90, reported: true, health: 'few' });
    expect(container.textContent).toContain('Few seeders');
    expect(container.textContent).toContain('2 seeders · 90 leechers');
    expect(getByRole('img').getAttribute('aria-label')).toBe('Seeders 2% of the swarm, 45 leechers per seeder');
  });

  it('says when leechers or the whole swarm were not reported', () => {
    const one = render(SwarmHealth, { seeders: 30, reported: true, health: 'good' });
    expect(one.container.textContent).toContain('leechers not reported');
    expect(one.container.querySelector('[role="img"]')).toBeNull();
    const none = render(SwarmHealth, { seeders: 0, reported: false, health: 'unknown' });
    expect(none.container.textContent?.trim()).toBe('Swarm not reported');
  });
});
