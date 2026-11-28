import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import DesirabilityGauge, { desirabilityParts } from './DesirabilityGauge.svelte';

describe('DesirabilityGauge', () => {
  it('names the side, the score and the parts in words', () => {
    const d = { score: 55, position: 'below' as const, qualitySteps: -1, cutoff: 'Bluray-1080p', sizeDelta: -0.5, preferredSize: 9.5e9, formatShortfall: 40, formatTarget: 100 };
    const { container } = render(DesirabilityGauge, { desirability: d, parts: true });
    expect(container.textContent).toContain('Below target');
    expect(container.textContent).toContain('55/100');
    expect(desirabilityParts(d)).toEqual([
      '1 step below the cutoff quality (Bluray-1080p)',
      '50% smaller than the preferred size (9.5 GB)',
      'Custom formats 40 points below the target of 100',
    ]);
  });

  it('covers on target, size unknown and outside the profile', () => {
    expect(desirabilityParts({ score: 100, position: 'on_target', qualitySteps: 0, cutoff: 'X', sizeDelta: 0.05, preferredSize: 1e9 }))
      .toEqual(['At the cutoff quality (X)', 'Near the preferred size (1.0 GB)']);
    expect(desirabilityParts({ score: 50, position: 'above', qualitySteps: 2, cutoff: 'X' })).toEqual(['2 steps above the cutoff quality (X)', 'Size unknown']);
    expect(render(DesirabilityGauge, {}).container.textContent).toContain('Outside the profile');
  });
});
