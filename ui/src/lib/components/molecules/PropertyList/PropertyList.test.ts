import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import PropertyList from './PropertyList.svelte';

describe('PropertyList', () => {
  it('renders terms and values, em dash for missing, links, copy and the managed seal', () => {
    const { container, getByRole, getByLabelText } = render(PropertyList, {
      items: [
        { key: 'a', label: 'Downloader', value: 'qBittorrent', href: '#dl' },
        { key: 'b', label: 'Ratio', value: null },
        { key: 'c', label: 'Hash', value: 'abc123', copy: true },
        { key: 'd', label: 'Category', value: 'tv', managed: true },
        { key: 'e', label: 'Zero', value: 0 },
      ],
    });
    const dts = [...container.querySelectorAll('dt')].map((d) => d.textContent);
    expect(dts).toEqual(['Downloader', 'Ratio', 'Hash', 'Category', 'Zero']);
    expect(getByRole('link', { name: 'qBittorrent' }).getAttribute('href')).toBe('#dl');
    expect(getByLabelText('Not set').textContent).toBe('—');
    expect(getByRole('button', { name: /Copy/ })).toBeTruthy();
    expect(getByRole('img', { name: 'Managed by config' })).toBeTruthy();
    expect(container.querySelectorAll('dd')[4].textContent!.trim()).toBe('0');
  });
});
