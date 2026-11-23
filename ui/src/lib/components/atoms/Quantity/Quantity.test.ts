import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import Quantity from './Quantity.svelte';

describe('Quantity', () => {
  it('splits number and unit and keeps the raw value', () => {
    const { container } = render(Quantity, { kind: 'bytes', value: 4_700_000_000 });
    const data = container.querySelector('data')!;
    expect(data.getAttribute('value')).toBe('4700000000');
    expect(data.textContent).toBe('4.7 GB');
    expect(data.querySelector('.unit')!.textContent).toBe(' GB');
  });
  it('formats counts against a limit', () => {
    const { container } = render(Quantity, { kind: 'count', value: 12, limit: 13 });
    expect(container.textContent).toBe('12 of 13');
  });
});
