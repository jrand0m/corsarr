import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import TemplateInput, { renderTemplate } from './TemplateInput.svelte';

const known = new Set(['Series Title', 'Season', 'Episode Title']);

describe('renderTemplate', () => {
  it('fills tokens, pads numbers and reports unknown tokens', () => {
    expect(renderTemplate('{Series Title} - S{Season:00}', { 'Series Title': 'The Long Haul', Season: 2 }, known))
      .toEqual({ text: 'The Long Haul - S02', unknown: [] });
    expect(renderTemplate('{Series Title} {Air Date}', { 'Series Title': 'X' }, known)).toEqual({ text: 'X {Air Date}', unknown: ['Air Date'] });
    expect(renderTemplate('{Episode Title}', { 'Episode Title': '' }, known).text).toBe('');
  });
});

describe('TemplateInput', () => {
  const tokens = [{ token: 'Series Title', group: 'Series' }, { token: 'Season', group: 'Episode' }];
  const samples = [{ name: 'Single episode', values: { 'Series Title': 'The Long Haul', Season: 2 } }];

  it('renders live examples and flags unknown tokens as invalid', async () => {
    const { getByLabelText, container, getByText } = render(TemplateInput, { label: 'File name', value: '{Series Title}', tokens, samples });
    expect(container.querySelector('dd code')!.textContent).toBe('The Long Haul');
    const input = getByLabelText('File name');
    await fireEvent.input(input, { target: { value: '{Series Title} {Nope}' } });
    expect(getByText('Unknown token: {Nope}')).toBeTruthy();
    expect(input.getAttribute('aria-invalid')).toBe('true');
  });

  it('inserts a token at the caret', async () => {
    const onchange = vi.fn();
    const { getByLabelText, getByRole } = render(TemplateInput, { label: 'File name', value: 'AB', tokens, samples, onchange });
    const input = getByLabelText('File name') as HTMLInputElement;
    input.setSelectionRange(1, 1);
    await fireEvent.keyUp(input);
    await fireEvent.click(getByRole('button', { name: 'Insert a token' }));
    await fireEvent.click(getByRole('button', { name: 'Insert {Season}' }));
    expect(onchange).toHaveBeenLastCalledWith('A{Season}B');
    expect(input.selectionStart).toBe(9);
  });
});
