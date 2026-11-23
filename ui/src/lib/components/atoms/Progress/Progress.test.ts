import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import Progress from './Progress.svelte';
import Meter from '../Meter/Meter.svelte';

describe('Progress', () => {
  it('exposes determinate values', () => {
    const { getByRole } = render(Progress, { label: 'Download', value: 0.426 });
    const bar = getByRole('progressbar', { name: 'Download' });
    expect(bar.getAttribute('aria-valuenow')).toBe('43');
    expect(bar.getAttribute('aria-valuetext')).toBe('43%');
  });
  it('reads countable work as "12 of 13"', () => {
    const { getByRole } = render(Progress, { label: 'Files', done: 12, total: 13 });
    expect(getByRole('progressbar').getAttribute('aria-valuetext')).toBe('12 of 13');
  });
  it('omits values when indeterminate', () => {
    const { getByRole } = render(Progress, { label: 'Checking' });
    expect(getByRole('progressbar').hasAttribute('aria-valuenow')).toBe(false);
  });
});

describe('Meter', () => {
  it('picks the tone from thresholds and the good end', () => {
    const { container } = render(Meter, { label: 'Free', value: 3, max: 100, better: 'high', warnAt: 0.15, errorAt: 0.05 });
    expect(container.querySelector('.meter')!.classList.contains('error')).toBe(true);
  });
  it('is neutral inside the safe range', () => {
    const { container, getByRole } = render(Meter, { label: 'Used', value: 0.4, warnAt: 0.8 });
    expect(container.querySelector('.meter')!.classList.contains('neutral')).toBe(true);
    expect(getByRole('meter').getAttribute('aria-valuetext')).toBe('40%');
  });
});
