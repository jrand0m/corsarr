import { describe, it, expect } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import InlineCheck from './InlineCheck.svelte';
import UrlField from '../UrlField/UrlField.svelte';
import SecretField from '../SecretField/SecretField.svelte';
import TextField from '../TextField/TextField.svelte';

const snip = (html: string) => createRawSnippet(() => ({ render: () => html }));

describe('InlineCheck', () => {
  it('shows the badge, the sentence and capped rows', () => {
    const { getByText, queryByText } = render(InlineCheck, {
      family: 'check', value: 'warn', detail: 'Matches 1 of 3.', items: ['a', 'b', 'c'], maxItems: 2,
    });
    expect(getByText('Warning')).toBeTruthy();
    expect(getByText('Matches 1 of 3.')).toBeTruthy();
    expect(getByText('and 1 more')).toBeTruthy();
    expect(queryByText('c')).toBeNull();
  });

  it('renders nothing while idle', () => {
    const { container } = render(InlineCheck, { family: 'sourceCheck' });
    expect(container.querySelector('.ic')).toBeNull();
  });
});

describe('fields with an inline action and result', () => {
  for (const [name, C] of [['UrlField', UrlField], ['SecretField', SecretField], ['TextField', TextField]] as const) {
    it(`${name}: renders the action beside the input, links the result and reports paste, input and blur`, async () => {
      const seen: string[] = [];
      const { getByLabelText, getByRole, container } = render(C as typeof TextField, {
        label: 'Field', action: snip('<button>Test</button>'), result: snip('<span>ok</span>'),
        onpaste: () => seen.push('paste'), oninput: (v: string) => seen.push(`input:${v}`), onblur: () => seen.push('blur'),
      });
      const input = getByLabelText('Field') as HTMLInputElement;
      expect(getByRole('button', { name: 'Test' })).toBeTruthy();
      const result = container.querySelector('.result')!;
      expect(input.getAttribute('aria-describedby')).toContain(result.id);
      expect(result.getAttribute('aria-live')).toBe('polite');
      await fireEvent.paste(input);
      await fireEvent.input(input, { target: { value: 'x' } });
      await fireEvent.blur(input);
      expect(seen).toEqual(['paste', 'input:x', 'blur']);
    });
  }
});
