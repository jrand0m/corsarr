import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import CodeViewer, { tokenizeLine } from './CodeViewer.svelte';

describe('tokenizeLine', () => {
  it('finds YAML keys and comments', () => {
    expect(tokenizeLine('  category: tv  # per kind', 'yaml')).toEqual([
      { text: '  ', kind: 'plain' }, { text: 'category', kind: 'key' }, { text: ':', kind: 'punct' },
      { text: ' tv ', kind: 'plain' }, { text: ' # per kind', kind: 'comment' },
    ]);
    expect(tokenizeLine('url: http://x:8080', 'yaml').map((t) => t.kind)).toEqual(['key', 'punct', 'plain']);
  });
  it('finds env names and JSON keys', () => {
    expect(tokenizeLine('export A_B=1', 'env')[1]).toEqual({ text: 'A_B', kind: 'key' });
    expect(tokenizeLine('  "event": "x#y"', 'json').map((t) => t.kind)).toEqual(['plain', 'key', 'punct', 'plain']);
  });
});

describe('CodeViewer', () => {
  it('numbers lines, names its focusable region and toggles wrap', async () => {
    const { getByRole, container } = render(CodeViewer, { code: 'a: 1\nb: 2\n', language: 'yaml', filename: 'corsarr.yaml' });
    const region = getByRole('region', { name: 'corsarr.yaml' });
    expect(region.getAttribute('tabindex')).toBe('0');
    expect([...container.querySelectorAll('.no')].map((n) => n.textContent)).toEqual(['1', '2']);
    const wrap = getByRole('button', { name: 'Wrap long lines' });
    await fireEvent.click(wrap);
    expect(container.querySelector('pre')!.classList.contains('wrap')).toBe(true);
  });

  it('copies and announces', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
    const { getByRole, findByText } = render(CodeViewer, { code: 'X=1', language: 'env' });
    await fireEvent.click(getByRole('button', { name: 'Copy' }));
    expect(writeText).toHaveBeenCalledWith('X=1');
    expect(await findByText('Copied to clipboard')).toBeTruthy();
  });
});
