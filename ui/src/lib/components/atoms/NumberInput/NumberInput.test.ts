import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import NumberInput from './NumberInput.svelte';

describe('NumberInput', () => {
  it('steps with the + and − buttons within limits', async () => {
    const onvaluechange = vi.fn();
    const { getByLabelText, getByRole } = render(NumberInput, { value: 9, max: 10, 'aria-label': 'Limit', onvaluechange });
    await fireEvent.click(getByLabelText('Increase'));
    expect(onvaluechange).toHaveBeenLastCalledWith(10);
    expect((getByLabelText('Increase') as HTMLButtonElement).disabled).toBe(true);
    await fireEvent.click(getByLabelText('Decrease'));
    expect((getByRole('spinbutton') as HTMLInputElement).value).toBe('9');
  });

  it('clamps typed values on blur and keeps step precision', async () => {
    const onvaluechange = vi.fn();
    const { getByRole } = render(NumberInput, { value: 1, min: 0, max: 5, step: 0.5, 'aria-label': 'Ratio', onvaluechange });
    const input = getByRole('spinbutton') as HTMLInputElement;
    await fireEvent.input(input, { target: { value: '7.25' } });
    await fireEvent.blur(input);
    expect(onvaluechange).toHaveBeenLastCalledWith(5);
  });

  it('allows empty as null', async () => {
    const onvaluechange = vi.fn();
    const { getByRole } = render(NumberInput, { value: 3, 'aria-label': 'Optional', onvaluechange });
    const input = getByRole('spinbutton');
    await fireEvent.input(input, { target: { value: '' } });
    await fireEvent.blur(input);
    expect(onvaluechange).not.toHaveBeenCalledWith(expect.any(Number));
  });
});
