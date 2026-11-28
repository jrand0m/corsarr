import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render } from '@testing-library/svelte';
import PluginInstallDialog, { grows } from './PluginInstallDialog.svelte';

const perms = { http: { hosts: ['$setting:url', 'api.dock.example'] }, secrets: ['token'] };

describe('PluginInstallDialog', () => {
  it('asks to install with the permissions', async () => {
    const onconfirm = vi.fn();
    const { getByRole, container } = render(PluginInstallDialog, { open: true, modal: false, change: 'install', name: 'Dock', version: '1.0.2', permissions: perms, labels: { url: 'Address' }, onconfirm });
    expect(getByRole('heading', { name: 'Install Dock 1.0.2?' })).toBeTruthy();
    expect(container.textContent).toContain('May contact the host in the Address setting');
    expect(container.textContent).toContain('May read the token secret');
    await fireEvent.click(getByRole('button', { name: 'Install' }));
    expect(onconfirm).toHaveBeenCalledOnce();
  });

  it('asks for approval when an update grows the permissions', () => {
    const diff = { addedHosts: ['api.dock.example'], removedHosts: [], addedSecrets: [], removedSecrets: [] };
    const { getByRole, container } = render(PluginInstallDialog, { open: true, modal: false, change: 'update', name: 'Dock', version: '1.1.0', installed: '1.0.2', permissions: perms, diff });
    expect(container.textContent).toContain('This version asks for more');
    expect(container.textContent).toContain('Installed now: 1.0.2');
    expect(getByRole('button', { name: 'Approve and update' })).toBeTruthy();
    expect(grows({ ...diff, addedHosts: [] })).toBe(false);
  });

  it('lists the instances an uninstall stops', () => {
    const usedBy = [{ section: 'downloadClients', key: 'up', name: 'Upstairs', enabled: true }];
    const { getByRole } = render(PluginInstallDialog, { open: true, modal: false, change: 'uninstall', name: 'Dock', usedBy });
    expect(getByRole('list', { name: 'Instances that stop' }).textContent).toContain('Upstairs');
    expect(getByRole('list', { name: 'Instances that stop' }).textContent).toContain('downloadClients.up');
    expect(getByRole('button', { name: 'Uninstall' })).toBeTruthy();
  });

  it('keeps the change button off when the config locks module changes', () => {
    const { getByRole, container } = render(PluginInstallDialog, { open: true, modal: false, change: 'install', name: 'Dock', version: '1', lock: { key: 'plugins.uiInstall', kind: 'env', name: 'CORSARR_PLUGINS_UIINSTALL' } });
    expect((getByRole('button', { name: 'Install' }) as HTMLButtonElement).disabled).toBe(true);
    expect(container.textContent).toContain('Module changes are off');
    expect(container.querySelector('[title]')!.getAttribute('title')).toContain('CORSARR_PLUGINS_UIINSTALL');
  });

  it('shows busy and the server refusal', () => {
    const { getByRole, container } = render(PluginInstallDialog, { open: true, modal: false, change: 'install', name: 'Dock', version: '1', busy: true, error: 'checksum does not match' });
    expect((getByRole('button', { name: 'Cancel' }) as HTMLButtonElement).disabled).toBe(true);
    expect(container.textContent).toContain('checksum does not match');
  });
});
