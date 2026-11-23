import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import AccessPage from './AccessPage.svelte';

describe('AccessPage', () => {
  it('renders one main column with the title, form and footer', () => {
    const children = createRawSnippet(() => ({ render: () => '<form aria-label="Sign in"><input aria-label="Password" /></form>' }));
    const footer = createRawSnippet(() => ({ render: () => '<p>Signed in by your reverse proxy</p>' }));
    const { getByRole, getByText } = render(AccessPage, { title: 'Sign in to corsarr', children, footer });
    const main = getByRole('main');
    expect(main.querySelector('h1')!.textContent).toBe('Sign in to corsarr');
    expect(getByRole('form', { name: 'Sign in' })).toBeTruthy();
    expect(getByText('Signed in by your reverse proxy')).toBeTruthy();
    expect(getByRole('banner').textContent).toContain('corsarr');
  });
});
