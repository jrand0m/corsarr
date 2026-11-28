import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, waitFor } from '@testing-library/svelte';
import PluginSettingsForm from './PluginSettingsForm.svelte';
import { harbourSchema, hostileSchema, hostileTexts, unsupportedSchema } from '../../../storybook/plugin-module-fixtures';
import { instanceMeta } from '../../../forms/plugin-schema';
import type { ModuleSchema } from '../../../forms/plugin-types';

describe('PluginSettingsForm', () => {
  it('renders every type of the closed set with groups', () => {
    const { getByLabelText, getByRole } = render(PluginSettingsForm, { schema: harbourSchema, label: 'New Harbour instance' });
    for (const name of ['Address', 'Username', 'Category', 'Parallel downloads', 'Seed ratio']) expect(getByLabelText(new RegExp(`^${name}`))).toBeTruthy();
    expect(getByRole('checkbox', { name: /Add paused/ })).toBeTruthy();
    expect(getByRole('group', { name: 'Connection' })).toBeTruthy();
    expect(getByRole('group', { name: /Path mappings/ })).toBeTruthy();
    expect(getByRole('option', { name: 'High ⚡' })).toBeTruthy();
  });

  it('shows a configured instance read-only with the lock and config values', () => {
    const meta = instanceMeta(harbourSchema.fields, 'downloadClients', 'upstairs', { apikey: 'CORSARR_SECRET_DOWNLOADCLIENTS_UPSTAIRS_APIKEY' });
    const { getByLabelText, container, queryByRole } = render(PluginSettingsForm, {
      schema: harbourSchema, values: { url: 'http://box:8080', slots: '6' }, meta, label: 'Upstairs', readonly: true,
    });
    expect((getByLabelText(/^Address/) as HTMLInputElement).value).toBe('http://box:8080');
    expect((getByLabelText(/^Parallel downloads/) as HTMLInputElement).value).toBe('6');
    expect(container.textContent).toContain('downloadClients.upstairs.url');
    expect(queryByRole('button', { name: 'Check settings' })).toBeNull();
  });

  it('runs an action with the current values and shows the answer as text', async () => {
    const onaction = vi.fn(async () => ({ outcome: 'ok' as const, text: 'Connected <b>2.4</b>' }));
    const { getByRole, getByLabelText } = render(PluginSettingsForm, { schema: harbourSchema, label: 'Draft', onaction, onsubmit: async () => [] });
    await fireEvent.input(getByLabelText(/^Address/), { target: { value: 'http://box:8080' } });
    await fireEvent.click(getByRole('button', { name: 'Test connection' }));
    expect(onaction).toHaveBeenCalledWith('test', expect.objectContaining({ url: 'http://box:8080', slots: 4, timeout: '30s', category: 'corsarr' }));
    await waitFor(() => expect(document.body.textContent).toContain('Connected <b>2.4</b>'));
    expect(document.querySelector('b')).toBeNull();
  });

  it('shows refused answers and field problems from an action', async () => {
    const onaction = vi.fn().mockResolvedValueOnce({ refused: true }).mockResolvedValueOnce({ problems: [{ path: 'url', message: 'is required' }] });
    const { getByRole, container } = render(PluginSettingsForm, { schema: harbourSchema, label: 'Draft', onaction });
    await fireEvent.click(getByRole('button', { name: 'Test connection' }));
    await waitFor(() => expect(container.textContent).toContain('The plugin answered with text corsarr cannot show.'));
    await fireEvent.click(getByRole('button', { name: 'Find categories' }));
    await waitFor(() => expect(container.textContent).toContain('is required. For example http://10.0.0.20:8080'));
  });

  it('checks a draft on the server and shows its messages', async () => {
    const onsubmit = vi.fn(async () => [{ path: 'values.url', message: 'must be reachable' }, { path: 'zzz', message: 'unknown setting' }]);
    const { getByRole, getByLabelText, container } = render(PluginSettingsForm, { schema: harbourSchema, label: 'Draft', onsubmit, submitLabel: 'Check' });
    await fireEvent.input(getByLabelText(/^Address/), { target: { value: 'http://box:8080' } });
    await fireEvent.input(getByLabelText(/^API key/), { target: { value: 'k' } });
    await fireEvent.click(getByRole('button', { name: 'Check' }));
    await waitFor(() => expect(onsubmit).toHaveBeenCalled());
    await waitFor(() => expect(container.textContent).toContain('must be reachable'));
    expect(container.textContent).toContain('zzz: unknown setting');
  });

  it('shows nothing from a schema outside the closed set', () => {
    const { container } = render(PluginSettingsForm, { schema: unsupportedSchema, label: 'Broken' });
    expect(container.textContent).toContain("This plugin's settings cannot be shown");
    expect(container.textContent).not.toContain('Body');
    expect(container.querySelector('form')).toBeNull();
  });

  it('renders hostile text literally', () => {
    const { container } = render(PluginSettingsForm, { schema: hostileSchema, label: 'Hostile', onaction: async () => ({ outcome: 'ok' as const, text: '' }) });
    expect(container.querySelector('img, script, iframe, style, a')).toBeNull();
    expect(container.textContent).toContain(hostileTexts[0]);
    expect(container.textContent).toContain(hostileTexts[9]);
  });
});

// ── seeded fuzz: random hostile texts in every text slot ────────────────────

function mulberry32(seed: number) {
  return () => {
    seed |= 0; seed = (seed + 0x6d2b79f5) | 0;
    let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const MARK = 'zq';
const pieces = [
  '<img src=x onerror=alert(1)>', '<script>alert(1)</script>', '"><svg onload=alert(1)>', "'><a href=\"javascript:alert(1)\">x</a>",
  '<style>*{color:red}</style>', '<iframe src="https://evil.example/f"></iframe>', 'javascript:alert(1)', 'https://evil.example/u',
  'data:text/html,<b>x</b>', '{' + '@html x}', '{#if 1}y{/if}', '&lt;i&gt;', '\u202E', '\u200B', 'url(https://evil.example/i)', '<b onclick=alert(1)>b</b>', '✅', '\n',
];

function hostile(rand: () => number): string {
  const n = 1 + Math.floor(rand() * 4);
  return MARK + Array.from({ length: n }, () => pieces[Math.floor(rand() * pieces.length)]).join(' ');
}

function randomSchema(rand: () => number): { schema: ModuleSchema; text: string } {
  const types = ['string', 'secret', 'bool', 'int', 'number', 'enum', 'duration', 'list<string>', 'list<int>', 'map<string,string>'];
  const fields = Array.from({ length: 2 + Math.floor(rand() * 6) }, (_, i) => {
    const type = types[Math.floor(rand() * types.length)];
    return {
      key: `f${i}`, type, label: hostile(rand), description: rand() < 0.7 ? hostile(rand) : undefined,
      options: type === 'enum' ? [{ value: 'a', label: hostile(rand) }, { value: 'b', label: hostile(rand) }] : undefined,
    };
  });
  return {
    schema: {
      fields,
      groups: rand() < 0.5 ? [{ title: hostile(rand), description: hostile(rand), fields: [fields[0].key] }] : undefined,
      actions: [{ id: 'go', label: hostile(rand) }],
    },
    text: hostile(rand),
  };
}

const urlAttrs = ['href', 'src', 'action', 'formaction', 'xlink:href', 'poster', 'srcdoc', 'style', 'background'];
const allowedTags = new Set(['DIV', 'FORM', 'FIELDSET', 'LEGEND', 'H3', 'P', 'SPAN', 'LABEL', 'INPUT', 'SELECT', 'OPTION', 'TEXTAREA', 'BUTTON', 'UL', 'LI', 'svg', 'path', 'circle', 'line', 'polyline', 'rect', 'polygon', 'g']);

describe('PluginSettingsForm fuzz', () => {
  it('never turns plugin text into an element, attribute or URL', async () => {
    const rand = mulberry32(20261127);
    for (let run = 0; run < 40; run++) {
      const { schema, text } = randomSchema(rand);
      const { container, unmount } = render(PluginSettingsForm, {
        schema, label: 'Fuzz', onaction: async () => ({ outcome: 'warn' as const, text }), onsubmit: async () => [],
        result: { action: schema.actions![0].label, state: 'ok', text },
      });
      for (const el of container.querySelectorAll('*')) {
        expect(allowedTags.has(el.tagName), `unexpected <${el.tagName}> in run ${run}`).toBe(true);
        for (const a of el.attributes) {
          expect(a.name.startsWith('on'), `event attribute ${a.name} in run ${run}`).toBe(false);
          if (urlAttrs.includes(a.name)) expect(a.value.includes(MARK) || /evil|javascript:|data:/.test(a.value), `${a.name}="${a.value}"`).toBe(false);
        }
      }
      expect(container.textContent).toContain(MARK);
      expect(container.textContent).toContain(text.slice(0, 12));
      unmount();
    }
  });
});
