import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import TextField from '../TextField/TextField.svelte';
import BoolField from '../BoolField/BoolField.svelte';
import NumberField from '../NumberField/NumberField.svelte';
import EnumField from '../EnumField/EnumField.svelte';

describe('Field (via TextField)', () => {
  it('labels the control and describes it with error, lock, drift and help in that order', () => {
    const { getByLabelText, container } = render(TextField, {
      label: 'Category', help: 'Where downloads land.', error: 'Already used', managed: 'downloaders.qbit.category',
      drift: 'Config says corsarr-tv',
    });
    const input = getByLabelText('Category') as HTMLInputElement;
    const ids = input.getAttribute('aria-describedby')!.split(' ');
    expect(ids.map((id) => container.querySelector(`[id="${id}"]`)!.textContent!.trim())).toEqual([
      'Already used',
      'Set in the config file at downloaders.qbit.category. Change it there; the form shows what is running.',
      'Config says corsarr-tv',
      'Where downloads land.',
    ]);
    expect(input.getAttribute('aria-invalid')).toBe('true');
    expect(input.readOnly).toBe(true);
    expect(container.textContent).toContain('Managed by config');
    expect(container.textContent).toContain('Drifted');
  });

  it('marks required fields visibly and on the control', () => {
    const { getByLabelText, getByText } = render(TextField, { label: 'Name', required: true });
    expect((getByLabelText('Name') as HTMLInputElement).required).toBe(true);
    expect(getByText('Required')).toBeTruthy();
  });

  it('has no describedby when there is nothing to describe', () => {
    const { getByLabelText } = render(TextField, { label: 'Name' });
    expect(getByLabelText('Name').hasAttribute('aria-describedby')).toBe(false);
  });
});

describe('simple field types', () => {
  it('BoolField: the checkbox carries the label and locks when managed', () => {
    const { getByRole } = render(BoolField, { label: 'Use SSL', checked: true, managed: true });
    const cb = getByRole('checkbox', { name: 'Use SSL' }) as HTMLInputElement;
    expect(cb.checked).toBe(true);
    expect(cb.disabled).toBe(true);
    expect(cb.getAttribute('aria-describedby')).toBeTruthy();
  });

  it('NumberField: labelled spinbutton with its value', () => {
    const { getByLabelText } = render(NumberField, { label: 'Port', value: 8080 });
    expect((getByLabelText('Port') as HTMLInputElement).value).toBe('8080');
  });

  it('EnumField radio: a labelled group whose locked state keeps only the chosen value enabled', () => {
    const { getByRole, getAllByRole } = render(EnumField, {
      label: 'Rule mode', value: 'live', appearance: 'radio', managed: true,
      options: [{ value: 'off', label: 'Off' }, { value: 'live', label: 'Live' }],
    });
    expect(getByRole('group', { name: /Rule mode/ })).toBeTruthy();
    const [off, live] = getAllByRole('radio') as HTMLInputElement[];
    expect(off.disabled).toBe(true);
    expect(live.disabled).toBe(false);
    expect(live.checked).toBe(true);
  });
});
