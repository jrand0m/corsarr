import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import StatusBadge from './StatusBadge.svelte';
import { vocabulary } from '../../../status/vocabulary';

describe('StatusBadge', () => {
  it('renders label and icon from the vocabulary, with the tone as a class', () => {
    const { container } = render(StatusBadge, { family: 'download', value: 'import-blocked' });
    const badge = container.querySelector('.badge')!;
    expect(badge.textContent).toContain('Import blocked');
    expect(badge.classList.contains('error')).toBe(true);
    expect(badge.querySelector('svg')).not.toBeNull();
  });

  it('appends the detail qualifier', () => {
    const { container } = render(StatusBadge, { family: 'health', value: 'failing', detail: 'timeout' });
    expect(container.textContent).toContain('Failing · timeout');
  });

  it('lets explicit props override the vocabulary', () => {
    const { container } = render(StatusBadge, { label: 'Custom', tone: 'managed', icon: 'lock' });
    expect(container.textContent).toContain('Custom');
    expect(container.querySelector('.managed')).not.toBeNull();
  });

  it('every vocabulary entry has a label, a tone and a known icon', () => {
    for (const fam of Object.values(vocabulary)) for (const e of Object.values(fam)) {
      expect(e.label.length).toBeGreaterThan(0);
      expect(['ok', 'warn', 'error', 'info', 'neutral', 'managed']).toContain(e.tone);
    }
  });

  it('explains kind modes in a tooltip and to screen readers', () => {
    const { container } = render(StatusBadge, { family: 'kindMode', value: 'shadow' });
    const badge = container.querySelector('.badge')!;
    expect(badge.getAttribute('title')).toMatch(/^Dry run:/);
    expect(badge.textContent).toContain('sends nothing to the download client');
  });

  it('shows no tooltip for values without a hint', () => {
    const { container } = render(StatusBadge, { family: 'download', value: 'queued' });
    expect(container.querySelector('.badge')!.hasAttribute('title')).toBe(false);
  });
});
