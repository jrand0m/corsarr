import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import PluginModulesView from './PluginModulesView.svelte';
import PluginModuleView from './PluginModuleView.svelte';
import PluginCatalogueView from './PluginCatalogueView.svelte';
import { catalogue, catalogueFailed, catalogueStale, harbourEnvExport, harbourModule, modules, policyLocked, policyOpen } from '../../../lib/storybook/plugin-module-fixtures';
import { parseEnvExport } from '../../model';
import { changeError, instanceViews, lockOf, permissionSummary } from './plugin-modules';

const fileSource = { kind: 'file', path: '/config/corsarr.yml' };

describe('plugin module helpers', () => {
  it('finds the lock and words refusals and permissions', () => {
    expect(lockOf(policyOpen, fileSource)).toBeUndefined();
    expect(lockOf(policyLocked, fileSource)).toEqual({ key: 'plugins.uiInstall', kind: 'file', file: '/config/corsarr.yml' });
    expect(changeError('catalogue_changed', 'x')).toContain('confirm again');
    expect(changeError('verify_failed', 'sha256 mismatch')).toBe('The file did not pass the checks: sha256 mismatch');
    expect(permissionSummary({ http: { hosts: ['a', 'b'] }, secrets: ['k'] })).toBe('2 hosts · 1 secret');
    expect(permissionSummary({ http: {} })).toBe('No access');
  });

  it('reads instance values and secret references from the config export', () => {
    const [v] = instanceViews(harbourModule, parseEnvExport(harbourEnvExport).instances);
    expect(v.values).toMatchObject({ url: 'http://10.0.0.20:8080', username: 'media', slots: '6', timeout: '1m', trackers: ['udp://tracker.example:1337'], paths: { downloads: '/data/downloads' } });
    expect(v.meta.apiKey).toEqual({ managed: 'downloadClients.upstairs.apiKey', isSet: true, ref: '${env:CORSARR_SECRET_DOWNLOADCLIENTS_UPSTAIRS_APIKEY}' });
  });
});

describe('PluginModulesView', () => {
  it('lists modules with status, version and what they may touch', () => {
    render(PluginModulesView, { modules, policy: policyOpen, source: fileSource });
    expect(screen.getAllByText('Harbour').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Failed to load').length).toBeGreaterThan(0);
    expect(screen.getAllByText('the module asked for 512 MiB; the limit is 64 MiB').length).toBeGreaterThan(0);
    expect(screen.getAllByText('1 host · 1 secret').length).toBeGreaterThan(0);
  });

  it('shows the lock with its source', () => {
    const { container } = render(PluginModulesView, { modules, policy: policyLocked, source: fileSource });
    expect(container.textContent).toContain('Module changes are off');
    expect(container.querySelector('[title*="plugins.uiInstall"]')!.getAttribute('title')).toContain('/config/corsarr.yml');
  });
});

describe('PluginModuleView', () => {
  const instances = instanceViews(harbourModule, parseEnvExport(harbourEnvExport).instances);

  it('shows the manifest as text with the homepage unlinked', () => {
    const { container } = render(PluginModuleView, { id: 'harbourdl', module: harbourModule, instances, policy: policyOpen });
    expect(container.textContent).toContain('Works with Harbour 2.0 and later ✅');
    expect(container.textContent).toContain('https://harbour.example/corsarr');
    expect(container.querySelector('a[href*="harbour.example"]')).toBeNull();
    expect(container.textContent).toContain('May contact the host in the Address setting');
  });

  it('shows each instance read-only with its config values and runs its actions', async () => {
    const onaction = vi.fn(async () => ({ outcome: 'ok' as const, text: 'Connected' }));
    render(PluginModuleView, { id: 'harbourdl', module: harbourModule, instances, policy: policyOpen, onaction });
    const card = screen.getByRole('article', { name: 'Upstairs box' });
    expect((within(card).getByLabelText(/^Address/) as HTMLInputElement).value).toBe('http://10.0.0.20:8080');
    await fireEvent.click(within(card).getByRole('button', { name: 'Test connection' }));
    await waitFor(() => expect(onaction).toHaveBeenCalledWith('test', 'downloadClients.upstairs', expect.any(Object)));
    await waitFor(() => expect(card.textContent).toContain('Connected'));
  });

  it('checks a draft on the server and shows the YAML with secret references', async () => {
    const onvalidate = vi.fn(async () => []);
    const { container } = render(PluginModuleView, { id: 'harbourdl', module: harbourModule, instances, policy: policyOpen, onvalidate });
    const draft = screen.getByRole('form', { name: 'New Harbour instance' });
    await fireEvent.input(within(screen.getByRole('region', { name: 'New instance' })).getByLabelText(/^Key\b/), { target: { value: 'cellar' } });
    await fireEvent.input(within(draft).getByLabelText(/^Address/), { target: { value: 'http://10.0.0.30:8080' } });
    await fireEvent.input(within(draft).getByLabelText(/^API key/), { target: { value: 'sk-secret' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Check and show YAML' }));
    await waitFor(() => expect(onvalidate).toHaveBeenCalledWith(expect.objectContaining({ url: 'http://10.0.0.30:8080', apiKey: 'sk-secret' })));
    await waitFor(() => expect(container.textContent).toContain('CORSARR_SECRET_DOWNLOADCLIENTS_CELLAR_APIKEY'));
    expect(container.textContent).toContain('downloadClients:');
    expect(container.textContent).not.toContain('sk-secret');
  });

  it('turns the change buttons off under the lock', () => {
    render(PluginModuleView, { id: 'harbourdl', module: harbourModule, instances, policy: policyLocked, source: fileSource });
    expect((screen.getByRole('button', { name: 'Uninstall' }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole('button', { name: 'Roll back' }) as HTMLButtonElement).disabled).toBe(true);
  });

  it('lists the instances an uninstall stops and reports a refusal', async () => {
    const onchange = vi.fn(async () => 'Module changes are off in the config.');
    render(PluginModuleView, { id: 'harbourdl', module: harbourModule, instances, policy: policyOpen, onchange });
    await fireEvent.click(screen.getByRole('button', { name: 'Uninstall' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(within(dialog).getByRole('list', { name: 'Instances that stop' }).textContent).toContain('Upstairs box');
    await fireEvent.click(within(dialog).getByRole('button', { name: 'Uninstall' }));
    await waitFor(() => expect(dialog.textContent).toContain('Module changes are off in the config.'));
  });
});

describe('PluginCatalogueView', () => {
  it('shows every offer state and installs the exact version', async () => {
    const oninstall = vi.fn(async () => {});
    const { container } = render(PluginCatalogueView, { catalogue, oninstall, now: new Date('2026-11-27T10:00:00Z') });
    for (const label of ['Available', 'Update available', 'Update needs approval', 'Installed', 'Newer installed', 'Incompatible']) {
      expect(container.textContent).toContain(label);
    }
    expect(container.textContent).toContain('needs corsarr ≥ 1.5.0');
    expect(container.querySelector('a[href*="dock.example"]')).toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: 'Install' }));
    const dialog = await screen.findByRole('dialog');
    await fireEvent.click(within(dialog).getByRole('button', { name: 'Install' }));
    await waitFor(() => expect(oninstall).toHaveBeenCalledWith(catalogue.offers![0]));
  });

  it('asks for approval on an update that grows its permissions', async () => {
    render(PluginCatalogueView, { catalogue });
    await fireEvent.click(screen.getByRole('button', { name: 'Review and update' }));
    const dialog = await screen.findByRole('dialog');
    expect(dialog.textContent).toContain('This version asks for more');
    expect(within(dialog).getByRole('button', { name: 'Approve and update' })).toBeTruthy();
  });

  it('shows a stale copy with its age and the error, or the error alone', () => {
    const r = render(PluginCatalogueView, { catalogue: catalogueStale, now: new Date('2026-11-27T10:00:00Z') });
    expect(r.container.textContent).toContain('Showing the last verified copy');
    expect(r.container.textContent).toContain('plugins.example did not answer within 10 s');
    r.unmount();
    const { container } = render(PluginCatalogueView, { catalogue: catalogueFailed });
    expect(container.textContent).toContain('The catalogue is not available');
    expect(container.textContent).toContain('the index signature does not match the pinned key');
  });

  it('turns install off under the lock', () => {
    render(PluginCatalogueView, { catalogue: { ...catalogue, policy: policyLocked }, source: { kind: 'env' } });
    expect((screen.getByRole('button', { name: 'Install' }) as HTMLButtonElement).disabled).toBe(true);
  });
});
