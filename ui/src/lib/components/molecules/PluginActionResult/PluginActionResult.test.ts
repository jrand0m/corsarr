import { describe, expect, it } from 'vitest';
import { render } from '@testing-library/svelte';
import PluginActionResult, { REFUSED_TEXT } from './PluginActionResult.svelte';

describe('PluginActionResult', () => {
  it('shows the outcome and the plugin text in a live region', () => {
    const { getByRole } = render(PluginActionResult, { state: 'warn', action: 'Find categories', text: 'Found 3.\nOne missing' });
    const s = getByRole('status');
    expect(s.textContent).toContain('Warning');
    expect(s.textContent).toContain('Find categories');
    expect(s.querySelector('.says')!.textContent).toBe('Found 3.\nOne missing');
  });

  it('shows running and refused with fixed wording', () => {
    const r = render(PluginActionResult, { state: 'running', action: 'Test connection', text: 'ignored' });
    expect(r.getByRole('status').textContent).toContain('Test connection…');
    expect(r.getByRole('status').textContent).not.toContain('ignored');
    r.unmount();
    const { getByRole } = render(PluginActionResult, { state: 'refused', text: '<b>x</b>' });
    expect(getByRole('status').textContent).toContain(REFUSED_TEXT);
    expect(getByRole('status').textContent).not.toContain('<b>x</b>');
  });

  it('never turns text into elements', () => {
    const { getByRole } = render(PluginActionResult, { state: 'ok', text: '<script>alert(1)</script><a href="javascript:x">y</a>' });
    const s = getByRole('status');
    expect(s.querySelector('script, a')).toBeNull();
    expect(s.textContent).toContain('<script>alert(1)</script>');
  });
});
