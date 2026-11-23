import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import Button from './Button.svelte';

const text = (t: string) => createRawSnippet(() => ({ render: () => `<span>${t}</span>` }));

describe('Button', () => {
  it('is a type=button by default and fires onclick', async () => {
    const onclick = vi.fn();
    const { getByRole } = render(Button, { children: text('Pause'), onclick });
    const btn = getByRole('button', { name: 'Pause' });
    expect(btn.getAttribute('type')).toBe('button');
    await fireEvent.click(btn);
    expect(onclick).toHaveBeenCalledOnce();
  });

  it('is busy and disabled while loading', () => {
    const { getByRole } = render(Button, { children: text('Save'), loading: true });
    const btn = getByRole('button');
    expect(btn.getAttribute('aria-busy')).toBe('true');
    expect((btn as HTMLButtonElement).disabled).toBe(true);
  });

  it('renders a link when given href', () => {
    const { getByRole } = render(Button, { children: text('Open health'), href: '#health' });
    expect(getByRole('link', { name: 'Open health' }).getAttribute('href')).toBe('#health');
  });
});
