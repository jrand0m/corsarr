import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import ConditionRow, { switchKind, type ConditionKindOption } from './ConditionRow.svelte';

const kinds: ConditionKindOption[] = [
  { kind: 'title', label: 'Release title', input: 'pattern' },
  { kind: 'audio', label: 'Audio codec', input: 'values', values: [{ value: 'aac', label: 'AAC' }, { value: 'dts', label: 'DTS' }] },
  { kind: 'size', label: 'Size', input: 'range', unit: 'GB' },
];

describe('ConditionRow', () => {
  it('switching kind keeps only the flags', () => {
    expect(switchKind({ kind: 'title', pattern: 'x', negate: true, required: true }, 'audio')).toEqual({ kind: 'audio', negate: true, required: true });
  });

  it('shows the input of the kind and reports changes', async () => {
    const onchange = vi.fn();
    const { getByLabelText, queryByLabelText } = render(ConditionRow, { value: { kind: 'title', pattern: '' }, kinds, position: 2, onchange });
    expect(queryByLabelText('Any of')).toBeNull();
    await fireEvent.input(getByLabelText('Pattern'), { target: { value: '\\bDTS\\b' } });
    expect(onchange).toHaveBeenLastCalledWith({ kind: 'title', pattern: '\\bDTS\\b' });
    await fireEvent.click(getByLabelText(/Negate/));
    expect(onchange).toHaveBeenLastCalledWith(expect.objectContaining({ negate: true }));
  });

  it('shows ranges with their unit, server errors and the test result; read-only hides remove', () => {
    const { getByLabelText, getByText, queryByRole } = render(ConditionRow, {
      value: { kind: 'size', min: 1, max: 10 }, kinds, position: 1, matched: true, errors: { max: 'must be above the minimum' }, readonly: true,
    });
    expect((getByLabelText(/From/) as HTMLInputElement).value).toBe('1');
    expect(getByText('must be above the minimum')).toBeTruthy();
    expect(getByText('Matches')).toBeTruthy();
    expect(queryByRole('button', { name: /Remove/ })).toBeNull();
  });
});
