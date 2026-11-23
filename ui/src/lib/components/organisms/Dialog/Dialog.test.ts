import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import { createRawSnippet, flushSync } from 'svelte';
import Dialog from './Dialog.svelte';

const body = createRawSnippet(() => ({ render: () => '<div><label>Root folder <input id="root" /></label><button id="b">Browse</button></div>' }));

function opener() {
  const btn = document.createElement('button');
  btn.textContent = 'Add series';
  document.body.append(btn);
  btn.focus();
  return btn;
}

describe('Dialog', () => {
  it('opens modally with title and description as its name and description', () => {
    const { getByRole } = render(Dialog, { open: true, title: 'Add The Long Haul', description: 'TV · 2024 · 3 seasons', children: body });
    const dlg = getByRole('dialog');
    expect(dlg.hasAttribute('open')).toBe(true);
    expect(dlg.getAttribute('aria-labelledby')).toBeTruthy();
    expect(document.getElementById(dlg.getAttribute('aria-labelledby')!)!.textContent).toBe('Add The Long Haul');
    expect(document.getElementById(dlg.getAttribute('aria-describedby')!)!.textContent).toBe('TV · 2024 · 3 seasons');
  });

  it('focuses the first field and returns focus to the opener on close', async () => {
    const btn = opener();
    const onclose = vi.fn();
    const { getByRole } = render(Dialog, { open: true, title: "Add", children: body, onclose });
    expect(document.activeElement?.id).toBe('root');
    await fireEvent.click(getByRole('button', { name: 'Close' }));
    expect(getByRole('dialog', { hidden: true }).hasAttribute('open')).toBe(false);
    expect(document.activeElement).toBe(btn);
    expect(onclose).toHaveBeenCalledWith('button');
  });

  it('closes on Escape (cancel) and on a scrim click, unless not dismissible', async () => {
    const onclose = vi.fn();
    const r = render(Dialog, { open: true, title: 'Add', children: body, onclose });
    const dlg = r.getByRole('dialog');
    await fireEvent(dlg, new Event('cancel', { cancelable: true }));
    expect(onclose).toHaveBeenLastCalledWith('escape');
    r.unmount();

    const r2 = render(Dialog, { open: true, title: 'Add', children: body, onclose });
    await fireEvent.click(r2.getByRole('dialog'));
    expect(onclose).toHaveBeenLastCalledWith('scrim');
    r2.unmount();

    onclose.mockClear();
    const r3 = render(Dialog, { open: true, title: 'Sign in again', children: body, onclose, dismissible: false });
    const d3 = r3.getByRole('dialog');
    const cancel = new Event('cancel', { cancelable: true });
    await fireEvent(d3, cancel);
    await fireEvent.click(d3);
    expect(cancel.defaultPrevented).toBe(true);
    expect(d3.hasAttribute('open')).toBe(true);
    expect(onclose).not.toHaveBeenCalled();
    expect(r3.queryByRole('button', { name: 'Close' })).toBeNull();
  });

  it('keeps Tab inside the dialog', async () => {
    const { getByRole } = render(Dialog, { open: true, title: 'Add', children: body });
    const dlg = getByRole('dialog');
    (document.getElementById('b') as HTMLElement).focus();
    await fireEvent.keyDown(dlg, { key: 'Tab' });
    expect(document.activeElement).toBe(getByRole('button', { name: 'Close' }));
  });

  it('opens and closes from the bound prop', async () => {
    const { getByRole, rerender } = render(Dialog, { open: false, title: 'Add', children: body });
    const dlg = getByRole('dialog', { hidden: true });
    expect(dlg.hasAttribute('open')).toBe(false);
    await rerender({ open: true });
    flushSync();
    expect(dlg.hasAttribute('open')).toBe(true);
    await rerender({ open: false });
    expect(dlg.hasAttribute('open')).toBe(false);
  });

  it('uses the alertdialog role when asked', () => {
    expect(render(Dialog, { open: true, title: 'Kill?', role: 'alertdialog' }).getByRole('alertdialog')).toBeTruthy();
  });
});
