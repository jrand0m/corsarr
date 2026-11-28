import { describe, expect, it } from 'vitest';
import { changeHow, settingsSections, sourceName } from './sections';

describe('settings sections', () => {
  it('lists the shipped sections with the instance count on plugins', () => {
    const s = settingsSections(3);
    expect(s.map((x) => x.id)).toEqual(['general', 'plugins', 'indexers', 'profiles', 'custom-formats']);
    expect(s.find((x) => x.id === 'plugins')?.count).toBe(3);
  });
});

describe('config source wording', () => {
  it('tells how to change config for each source', () => {
    expect(changeHow('file', '/config/corsarr.yml')).toBe('Edit /config/corsarr.yml; corsarr picks up the change on its own.');
    expect(changeHow('file')).toBe('Edit the config file; corsarr picks up the change on its own.');
    expect(changeHow('args')).toBe('Change the --section.key=value arguments corsarr starts with, then restart it.');
    expect(changeHow('env')).toBe('Change the CORSARR_* environment variables, then restart corsarr.');
    expect(changeHow(undefined)).toBe('Write a config file (download the export below as a start), point --configFile at it, then restart corsarr.');
  });

  it('names each source', () => {
    expect(sourceName('file', '/config/corsarr.yml')).toBe('the file /config/corsarr.yml');
    expect(sourceName('file')).toBe('the config file');
    expect(sourceName('args')).toBe('command-line arguments');
    expect(sourceName('env')).toBe('environment variables');
    expect(sourceName('defaults')).toBe('built-in defaults');
    expect(sourceName(undefined)).toBe('built-in defaults');
  });
});
