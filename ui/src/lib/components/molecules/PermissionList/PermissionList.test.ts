import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/svelte';
import PermissionList, { hostText } from './PermissionList.svelte';

describe('PermissionList', () => {
  it('words hosts, setting-bound hosts and secrets', () => {
    const { container } = render(PermissionList, {
      permissions: { http: { hosts: ['$setting:url', 'api.example.org', '*.cdn.example'] }, secrets: ['apiKey'] },
      labels: { url: 'Address', apiKey: 'API key' },
    });
    const text = container.textContent!;
    expect(text).toContain('May contact the host in the Address setting');
    expect(text).toContain('May contact api.example.org');
    expect(text).toContain('May contact any subdomain of cdn.example');
    expect(text).toContain('May read the API key secret');
    expect(text).not.toContain('New');
  });

  it('says when nothing is needed', () => {
    const { container } = render(PermissionList, { permissions: { http: {} } });
    expect(container.textContent).toContain('Needs no network access and reads no secrets.');
  });

  it('marks added entries and lists removed ones for an update', () => {
    const { getAllByText, getByText } = render(PermissionList, {
      permissions: { http: { hosts: ['a.example', 'b.example'] }, secrets: ['token'] },
      diff: { addedHosts: ['b.example'], removedHosts: ['old.example'], addedSecrets: ['token'], removedSecrets: [] },
    });
    expect(getAllByText('New')).toHaveLength(2);
    expect(getByText('old.example').closest('li')!.textContent).toContain('No longer needed');
  });

  it('keeps hostile names as text', () => {
    const { container } = render(PermissionList, { permissions: { http: { hosts: ['<img src=x onerror=alert(1)>'] } } });
    expect(container.querySelector('img')).toBeNull();
    expect(container.textContent).toContain('<img src=x onerror=alert(1)>');
  });

  it('formats host patterns', () => {
    expect(hostText('$setting:host')).toBe('the host in the host setting');
    expect(hostText('*.example.org')).toBe('any subdomain of example.org');
  });
});
