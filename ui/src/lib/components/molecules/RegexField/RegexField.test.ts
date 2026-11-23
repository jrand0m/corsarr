import { describe, it, expect } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import RegexField, { testRegex } from './RegexField.svelte';

describe('RegexField', () => {
  it('reports match position, no match, invalid and empty', () => {
    expect(testRegex('1080p', 'Show.S01E01.1080p.WEB', true)).toEqual({ kind: 'match', before: 'Show.S01E01.', match: '1080p', after: '.WEB' });
    expect(testRegex('CAM', 'Show.2160p', true)).toEqual({ kind: 'no-match' });
    expect(testRegex('(1080p', 'x', true).kind).toBe('invalid');
    expect(testRegex('', 'x', true)).toEqual({ kind: 'empty' });
    expect(testRegex('web', 'Show.WEB', false)).toEqual({ kind: 'no-match' });
  });

  it('tests live as the sample changes and flags a broken pattern as the field error', async () => {
    const { getByLabelText, container, getByText } = render(RegexField, { label: 'Must contain', value: '1080p' });
    await fireEvent.input(getByLabelText('Test text for Must contain'), { target: { value: 'Dockside.2026.1080p.WEB' } });
    expect(container.querySelector('mark')!.textContent).toBe('1080p');
    const pattern = getByLabelText('Must contain') as HTMLInputElement;
    expect(pattern.getAttribute('aria-describedby')).toContain(container.querySelector('.res')!.id);
    await fireEvent.input(pattern, { target: { value: '(1080p' } });
    expect(getByText(/does not compile/)).toBeTruthy();
    expect(pattern.getAttribute('aria-invalid')).toBe('true');
  });
});
