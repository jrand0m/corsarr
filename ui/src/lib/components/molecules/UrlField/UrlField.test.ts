import { describe, it, expect } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import UrlField, { urlProblem } from './UrlField.svelte';

describe('UrlField', () => {
  it('explains missing schemes and unsupported ones', () => {
    expect(urlProblem('')).toBeUndefined();
    expect(urlProblem('http://10.0.0.20:8080')).toBeUndefined();
    expect(urlProblem('10.0.0.20:8080')).toMatch(/full address/);
    expect(urlProblem('ftp://nas')).toMatch(/http:\/\/ or https:\/\//);
  });

  it('checks on blur, clears as the owner fixes it, and lets a server error win', async () => {
    const { getByLabelText, queryByText, rerender } = render(UrlField, { label: 'Address' });
    const input = getByLabelText('Address');
    await fireEvent.input(input, { target: { value: 'nas.local' } });
    expect(queryByText(/full address/)).toBeNull();
    await fireEvent.blur(input);
    expect(queryByText(/full address/)).toBeTruthy();
    await fireEvent.input(input, { target: { value: 'http://nas.local' } });
    expect(queryByText(/full address/)).toBeNull();
    await rerender({ label: 'Address', error: 'No answer within 5 s.' });
    expect(queryByText('No answer within 5 s.')).toBeTruthy();
  });
});
