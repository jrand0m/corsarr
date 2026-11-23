import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import ListField from './ListField.svelte';
import MapField, { duplicateKeys } from '../MapField/MapField.svelte';
import MultiselectField from '../MultiselectField/MultiselectField.svelte';

describe('ListField', () => {
  it('adds an entry, focuses it, edits and removes', async () => {
    const onchange = vi.fn();
    const { getByRole, getAllByRole } = render(ListField, { label: 'Ignored words', itemLabel: 'word', value: ['sample'], onchange });
    await fireEvent.click(getByRole('button', { name: 'Add word' }));
    expect(onchange).toHaveBeenLastCalledWith(['sample', '']);
    const inputs = getAllByRole('textbox');
    expect(document.activeElement).toBe(inputs[1]);
    await fireEvent.input(inputs[1], { target: { value: 'trailer' } });
    expect(onchange).toHaveBeenLastCalledWith(['sample', 'trailer']);
    await fireEvent.click(getByRole('button', { name: 'Remove word 1' }));
    expect(onchange).toHaveBeenLastCalledWith(['trailer']);
  });

  it('Enter in the last entry adds another; locked lists have no add or remove', async () => {
    const onchange = vi.fn();
    const { getAllByRole, rerender, queryByRole } = render(ListField, { label: 'Tags', itemLabel: 'tag', value: ['4k'], onchange });
    await fireEvent.keyDown(getAllByRole('textbox')[0], { key: 'Enter' });
    expect(onchange).toHaveBeenLastCalledWith(['4k', '']);
    await rerender({ label: 'Tags', itemLabel: 'tag', value: ['4k'], managed: true });
    expect(queryByRole('button', { name: 'Add tag' })).toBeNull();
  });
});

describe('MapField', () => {
  it('finds duplicate keys, ignoring blanks', () => {
    expect([...duplicateKeys([{ key: 'a', value: '1' }, { key: 'b', value: '' }, { key: 'a ', value: '2' }, { key: '', value: '' }, { key: '', value: '' }])]).toEqual([2, 0]);
  });

  it('adds rows and flags duplicates inline', async () => {
    const onchange = vi.fn();
    const { getByRole, getByLabelText, getByText } = render(MapField, {
      label: 'Category map', keyLabel: 'Category', valueLabel: 'Kind', value: [{ key: '5040', value: 'tv' }], onchange,
    });
    await fireEvent.click(getByRole('button', { name: 'Add row' }));
    await fireEvent.input(getByLabelText('Category 2'), { target: { value: '5040' } });
    expect(onchange).toHaveBeenLastCalledWith([{ key: '5040', value: 'tv' }, { key: '5040', value: '' }]);
    expect(getByText('This category appears more than once', { selector: '[id$="-d1"]' })).toBeTruthy();
    expect(getByLabelText('Category 1').getAttribute('aria-invalid')).toBe('true');
  });
});

describe('MultiselectField', () => {
  it('toggles values and counts them', async () => {
    const onchange = vi.fn();
    const options = [{ value: 'sd', label: 'TV SD' }, { value: 'hd', label: 'TV HD' }];
    const { getByRole, getByText } = render(MultiselectField, { label: 'Categories', value: ['sd'], options, onchange });
    expect(getByText('1 of 2 selected')).toBeTruthy();
    await fireEvent.click(getByRole('checkbox', { name: 'TV HD' }));
    expect(onchange).toHaveBeenLastCalledWith(['sd', 'hd']);
    await fireEvent.click(getByRole('checkbox', { name: 'TV SD' }));
    expect(onchange).toHaveBeenLastCalledWith(['hd']);
  });
});
