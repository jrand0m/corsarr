import { describe, it, expect } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import Checkbox from './Checkbox.svelte';
import Radio from '../Radio/Radio.svelte';
import TextInput from '../TextInput/TextInput.svelte';

describe('form atoms', () => {
  it('Checkbox is labelled, toggles and shows mixed state', async () => {
    const { getByRole } = render(Checkbox, { label: 'Season folders', indeterminate: true });
    const cb = getByRole('checkbox', { name: 'Season folders' }) as HTMLInputElement;
    expect(cb.indeterminate).toBe(true);
    await fireEvent.click(cb);
    expect(cb.checked).toBe(true);
  });
  it('Radio links its description', () => {
    const { getByRole } = render(Radio, { value: 'a', label: 'Remove only', description: 'Can be grabbed again.' });
    expect(getByRole('radio', { name: 'Remove only' }).getAttribute('aria-describedby')).toBeTruthy();
  });
  it('TextInput marks invalid input', () => {
    const { getByRole } = render(TextInput, { 'aria-label': 'URL', invalid: true });
    expect(getByRole('textbox', { name: 'URL' }).getAttribute('aria-invalid')).toBe('true');
  });
});
