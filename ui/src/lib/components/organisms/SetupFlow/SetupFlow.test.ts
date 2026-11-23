import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';
import SetupFlow from './SetupFlow.svelte';
import type { SetupFlowSpec } from '../../../forms/schema';

const pin: SetupFlowSpec = { kind: 'pinPairing', label: 'Pair with Plex', fills: ['token'] };
const login: SetupFlowSpec = { kind: 'loginForm', label: 'Sign in', fills: ['cookie'], otp: true };
const now = Date.parse('2026-11-22T14:05:00Z');

describe('SetupFlow', () => {
  it('shows a pairing code, waits, then reports the filled fields', async () => {
    let finish!: (v: { ok: true }) => void;
    const ondone = vi.fn();
    const driver = {
      start: vi.fn(async () => ({ code: 'K7QP', url: 'https://plex.tv/link', expiresAt: now + 600_000 })),
      wait: vi.fn(() => new Promise<{ ok: true }>((r) => (finish = r))),
    };
    const { getByRole, getByText } = render(SetupFlow, { flow: pin, driver, ondone, now });
    await fireEvent.click(getByRole('button', { name: 'Get a code' }));
    await waitFor(() => expect(getByText('K7QP')).toBeTruthy());
    expect(getByText('Waiting for confirmation')).toBeTruthy();
    expect(getByRole('link', { name: /plex\.tv\/link/ })).toBeTruthy();
    finish({ ok: true });
    await waitFor(() => expect(getByText('Connected')).toBeTruthy());
    expect(ondone).toHaveBeenCalledWith(['token'], undefined);
  });

  it('cancels a pending pairing and ignores its late result', async () => {
    let finish!: (v: { ok: true }) => void;
    const cancel = vi.fn();
    const ondone = vi.fn();
    const driver = { start: async () => ({ code: 'K7QP' }), wait: () => new Promise<{ ok: true }>((r) => (finish = r)), cancel };
    const { getByRole, queryByText } = render(SetupFlow, { flow: pin, driver, ondone });
    await fireEvent.click(getByRole('button', { name: 'Get a code' }));
    await waitFor(() => getByRole('button', { name: 'Cancel' }));
    await fireEvent.click(getByRole('button', { name: 'Cancel' }));
    expect(cancel).toHaveBeenCalled();
    finish({ ok: true });
    await Promise.resolve();
    expect(ondone).not.toHaveBeenCalled();
    expect(queryByText('Connected')).toBeNull();
  });

  it('submits credentials once and shows a refusal', async () => {
    const submit = vi.fn(async () => ({ ok: false as const, error: 'The tracker refused those credentials.' }));
    const { getByRole, getByLabelText } = render(SetupFlow, { flow: login, driver: { submit } });
    const signIn = getByRole('button', { name: 'Sign in' }) as HTMLButtonElement;
    expect(signIn.disabled).toBe(true);
    await fireEvent.input(getByLabelText('Username'), { target: { value: 'harbourmaster' } });
    await fireEvent.input(getByLabelText('Password'), { target: { value: 'hunter2' } });
    await fireEvent.input(getByLabelText('One-time code'), { target: { value: '123456' } });
    await fireEvent.click(signIn);
    expect(submit).toHaveBeenCalledWith(login, { username: 'harbourmaster', password: 'hunter2', otp: '123456' });
    await waitFor(() => expect(getByRole('alert').textContent).toBe('The tracker refused those credentials.'));
  });
});
