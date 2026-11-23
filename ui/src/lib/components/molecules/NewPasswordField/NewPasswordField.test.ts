import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import NewPasswordField, { passwordStrength } from './NewPasswordField.svelte';

describe('passwordStrength', () => {
  it('scores length first, then variety, and penalises repeats', () => {
    expect(passwordStrength('').score).toBe(0);
    expect(passwordStrength('harbour')).toMatchObject({ score: 1, label: 'Too short', hint: '5 more characters needed.' });
    expect(passwordStrength('harbourmaster').score).toBe(2);
    expect(passwordStrength('harbourmaster-quay').score).toBe(3);
    expect(passwordStrength('Quay-lights-7-after-dark')).toMatchObject({ score: 4, label: 'Strong' });
    expect(passwordStrength('aaaaaaaaaaaaaaaa').score).toBe(2);
  });
});

describe('NewPasswordField', () => {
  it('shows strength, toggles visibility of both inputs and reports validity', async () => {
    const onchange = vi.fn();
    const { getByLabelText, getByRole } = render(NewPasswordField, { onchange });
    const pw = getByLabelText(/^Password(?! strength)/) as HTMLInputElement;
    const confirm = getByLabelText(/^Confirm password/) as HTMLInputElement;
    expect(pw.autocomplete).toBe('new-password');
    await fireEvent.input(pw, { target: { value: 'Quay-lights-7-after-dark' } });
    expect(getByRole('meter', { name: 'Password strength' }).getAttribute('aria-valuetext')).toBe('Strong');
    expect(onchange).toHaveBeenLastCalledWith({ value: 'Quay-lights-7-after-dark', valid: false });
    await fireEvent.input(confirm, { target: { value: 'Quay-lights-7-after-dark' } });
    expect(onchange).toHaveBeenLastCalledWith({ value: 'Quay-lights-7-after-dark', valid: true });
    await fireEvent.click(getByRole('button', { name: 'Show password' }));
    expect(pw.type).toBe('text');
    expect(confirm.type).toBe('text');
  });

  it('reports a mismatch once the confirm is as long as the password or left', async () => {
    const { getByLabelText, queryByText } = render(NewPasswordField, { value: 'harbourmaster' });
    const confirm = getByLabelText(/^Confirm password/);
    await fireEvent.input(confirm, { target: { value: 'harb' } });
    expect(queryByText('The passwords do not match')).toBeNull();
    await fireEvent.blur(confirm);
    expect(queryByText('The passwords do not match')).toBeTruthy();
    expect(confirm.getAttribute('aria-invalid')).toBe('true');
  });

  it('shows required errors after a submit attempt', () => {
    const { getByText } = render(NewPasswordField, { showErrors: true });
    expect(getByText('Password is required')).toBeTruthy();
  });
});
