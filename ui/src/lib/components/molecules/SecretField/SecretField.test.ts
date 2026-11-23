import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import SecretField, { parseSecretRef } from './SecretField.svelte';

describe('SecretField', () => {
  it('parses env and file references', () => {
    expect(parseSecretRef('${env:NZBGEEK_API_KEY}')).toEqual({ kind: 'env', target: 'NZBGEEK_API_KEY' });
    expect(parseSecretRef('${file:/run/secrets/qbit}')).toEqual({ kind: 'file', target: '/run/secrets/qbit' });
    expect(parseSecretRef('plain')).toBeUndefined();
  });

  it('never shows a stored value: masked input, "Set" state and a replace placeholder', () => {
    const { getByLabelText, getByText } = render(SecretField, { label: 'API key', isSet: true });
    const input = getByLabelText('API key') as HTMLInputElement;
    expect(input.type).toBe('password');
    expect(input.value).toBe('');
    expect(input.placeholder).toMatch(/type to replace/);
    expect(getByText('Set')).toBeTruthy();
  });

  it('reveals only what was typed', async () => {
    const onchange = vi.fn();
    const { getByLabelText, getByRole } = render(SecretField, { label: 'API key', onchange });
    const reveal = getByRole('button', { name: 'Show what you typed' }) as HTMLButtonElement;
    expect(reveal.disabled).toBe(true);
    const input = getByLabelText('API key') as HTMLInputElement;
    await fireEvent.input(input, { target: { value: 'abc123' } });
    await fireEvent.click(reveal);
    expect(input.type).toBe('text');
    expect(getByRole('button', { name: 'Hide what you typed' }).getAttribute('aria-pressed')).toBe('true');
  });

  it('shows a reference instead of the input and can switch to a value', async () => {
    const { getByLabelText, getByRole, getByText } = render(SecretField, { label: 'API key', ref: '${env:NZBGEEK_API_KEY}' });
    expect((getByLabelText('API key') as HTMLInputElement).value).toBe('env: NZBGEEK_API_KEY');
    expect(getByText('From reference')).toBeTruthy();
    await fireEvent.click(getByRole('button', { name: 'Use a value instead' }));
    expect((getByLabelText('API key') as HTMLInputElement).type).toBe('password');
    await fireEvent.click(getByRole('button', { name: 'Keep reference' }));
    expect((getByLabelText('API key') as HTMLInputElement).readOnly).toBe(true);
  });

  it('offers no switch away from a managed reference', () => {
    const { queryByRole } = render(SecretField, { label: 'API key', ref: '${env:X}', managed: true });
    expect(queryByRole('button', { name: 'Use a value instead' })).toBeNull();
  });
});
