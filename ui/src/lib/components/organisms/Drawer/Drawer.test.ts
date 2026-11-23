import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import Drawer from './Drawer.svelte';

const body = createRawSnippet(() => ({ render: () => '<div><a href="#files">Files</a><button id="act">Recheck</button></div>' }));
const footer = createRawSnippet(() => ({ render: () => '<div><button id="primary" data-autofocus>Open import</button></div>' }));

describe('Drawer', () => {
  it('opens as a labelled modal sheet and focuses the marked element', () => {
    const { getByRole } = render(Drawer, { open: true, title: 'The Long Haul S02E03', subtitle: 'qBittorrent · tv-corsarr', children: body, footer });
    const sheet = getByRole('dialog', { name: 'The Long Haul S02E03' });
    expect(sheet.hasAttribute('open')).toBe(true);
    expect(sheet.textContent).toContain('qBittorrent · tv-corsarr');
    expect(document.activeElement?.id).toBe('primary');
  });

  it('closes on Escape, scrim click and the close button, returning focus to the row', async () => {
    const row = document.createElement('button');
    document.body.append(row);
    const onclose = vi.fn();
    for (const how of ['escape', 'scrim', 'button'] as const) {
      row.focus();
      const r = render(Drawer, { open: true, title: 'Dockside', children: body, onclose });
      const sheet = r.getByRole('dialog');
      if (how === 'escape') await fireEvent(sheet, new Event('cancel', { cancelable: true }));
      if (how === 'scrim') await fireEvent.click(sheet);
      if (how === 'button') await fireEvent.click(r.getByRole('button', { name: 'Close details' }));
      expect(sheet.hasAttribute('open')).toBe(false);
      expect(onclose).toHaveBeenLastCalledWith(how);
      expect(document.activeElement).toBe(row);
      r.unmount();
    }
  });

  it('does not close when clicking inside the sheet', async () => {
    const { getByRole } = render(Drawer, { open: true, title: 'Dockside', children: body });
    await fireEvent.click(getByRole('button', { name: 'Recheck' }));
    expect(getByRole('dialog').hasAttribute('open')).toBe(true);
  });

  it('wraps Tab from the last control to the first', async () => {
    const { getByRole } = render(Drawer, { open: true, title: 'Dockside', children: body });
    getByRole('button', { name: 'Recheck' }).focus();
    await fireEvent.keyDown(getByRole('dialog'), { key: 'Tab' });
    expect(document.activeElement).toBe(getByRole('button', { name: 'Close details' }));
  });
});
