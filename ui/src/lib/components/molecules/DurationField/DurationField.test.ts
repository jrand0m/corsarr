import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import DurationField from './DurationField.svelte';
import SizeField from '../SizeField/SizeField.svelte';

describe('DurationField', () => {
  it('splits seconds into the largest exact unit and shows the compact reading', () => {
    const { getByLabelText, container } = render(DurationField, { label: 'Stall grace', value: 5400 });
    expect((getByLabelText('Stall grace') as HTMLInputElement).value).toBe('90');
    expect((getByLabelText('Stall grace: unit') as HTMLSelectElement).value).toBe('m');
    expect(container.querySelector('output')!.textContent).toBe('1h 30m');
  });

  it('commits amount × unit back to seconds', async () => {
    const onchange = vi.fn();
    const { getByLabelText } = render(DurationField, { label: 'Retry after', value: 3600, onchange });
    const unit = getByLabelText('Retry after: unit') as HTMLSelectElement;
    expect(unit.value).toBe('h');
    unit.value = 'd';
    await fireEvent.change(unit);
    expect(onchange).toHaveBeenLastCalledWith(86400);
    const amount = getByLabelText('Retry after') as HTMLInputElement;
    await fireEvent.input(amount, { target: { value: '2' } });
    expect(amount.value).toBe('2');
    await fireEvent.blur(amount);
    expect(onchange).toHaveBeenLastCalledWith(172800);
  });
});

describe('SizeField', () => {
  it('edits bytes in SI units', async () => {
    const onchange = vi.fn();
    const { getByLabelText } = render(SizeField, { label: 'Maximum size', value: 4_000_000_000, onchange });
    const amount = getByLabelText('Maximum size') as HTMLInputElement;
    expect(amount.value).toBe('4');
    await fireEvent.input(amount, { target: { value: '4.7' } });
    await fireEvent.blur(amount);
    expect(onchange).toHaveBeenLastCalledWith(4_700_000_000);
  });
});
