import { describe, expect, it } from 'vitest';
import { harbourModule, policyOpen } from '../../../lib/storybook/plugin-module-fixtures';
import type { ModulePermissions, ModuleSchema } from '../../../lib/forms/plugin-types';
import type { ConfiguredInstance } from '../../model';
import { changeError, instanceViews, lockOf, permissionSummary, problemsOf, sortModules } from './plugin-modules';

describe('refused module changes', () => {
  it.each([
    ['plugins_locked', 'Set plugins.uiInstall.', 'Module changes are off in the config. Set plugins.uiInstall.'],
    ['catalogue_changed', 'x', 'The catalogue now offers another version. Check the new entry and confirm again.'],
    ['verify_failed', 'sha256 mismatch', 'The file did not pass the checks: sha256 mismatch'],
    ['incompatible', 'needs ABI 2', 'This version does not run on this corsarr: needs ABI 2'],
    ['no_previous', 'x', 'There is no previous version to go back to.'],
    ['modules_unavailable', 'x', 'Plugin modules are switched off on this server.'],
    ['catalogue_unreachable', 'timeout', 'The catalogue is not reachable: timeout'],
    ['something_else', 'The server said no.', 'The server said no.'],
  ])('words %s', (code, message, want) => {
    expect(changeError(code, message)).toBe(want);
  });

  it('reads the problems of a refused install, or none', () => {
    const problems = [{ path: 'settings.fields.0', message: 'unknown type' }];
    expect(problemsOf({ problems })).toEqual(problems);
    expect(problemsOf({ problems: 'not a list' })).toEqual([]);
    expect(problemsOf(undefined)).toEqual([]);
  });

  it('leaves module changes open when there is no policy or the UI may install', () => {
    expect(lockOf(undefined)).toBeUndefined();
    expect(lockOf(policyOpen)).toBeUndefined();
  });
});

describe('permission summary', () => {
  it.each([
    [undefined, 'No access'],
    [{}, 'No access'],
    [{ http: { hosts: [] }, secrets: [] }, 'No access'],
    [{ http: { hosts: ['a'] } }, '1 host'],
    [{ http: { hosts: ['a', 'b'] } }, '2 hosts'],
    [{ secrets: ['k'] }, '1 secret'],
    [{ secrets: ['k', 'l', 'm'] }, '3 secrets'],
    [{ http: { hosts: ['a'] }, secrets: ['k', 'l'] }, '1 host · 2 secrets'],
  ])('%j reads %s', (p, want) => {
    // Manifests may leave out either permission list.
    expect(permissionSummary(p as ModulePermissions | undefined)).toBe(want);
  });
});

describe('instance views', () => {
  const configured: ConfiguredInstance[] = [{
    section: 'downloadClients', key: 'upstairs', plugin: 'harbourdl',
    values: { url: 'http://box:8080', slots: '6' }, secrets: {},
  }];

  it('keeps only the fields the instance sets', () => {
    const [v] = instanceViews(harbourModule, configured);
    expect(v.ref.key).toBe('upstairs');
    expect(v.values).toEqual({ url: 'http://box:8080', slots: '6' });
  });

  it('shows an instance missing from the export with no values', () => {
    const [v] = instanceViews(harbourModule, []);
    expect(v.ref.key).toBe('upstairs');
    expect(v.values).toEqual({});
  });

  it('shows no instances for an unused module and no values for a module without settings', () => {
    expect(instanceViews({ ...harbourModule, usedBy: null }, configured)).toEqual([]);
    const [v] = instanceViews({ ...harbourModule, manifest: { ...harbourModule.manifest, settings: undefined as unknown as ModuleSchema } }, configured);
    expect(v.values).toEqual({});
    expect(v.meta).toEqual({});
  });
});

describe('module order', () => {
  it('sorts by display name, falling back to the id', () => {
    const named = (id: string, name: string) => ({ ...harbourModule, id, manifest: { ...harbourModule.manifest, name } });
    expect(sortModules([named('b', 'Zed'), named('alpha', ''), named('c', 'Bay')]).map((m) => m.id)).toEqual(['alpha', 'c', 'b']);
  });
});
