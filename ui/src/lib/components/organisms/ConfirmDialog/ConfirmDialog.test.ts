import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import ConfirmDialog from './ConfirmDialog.svelte';

const kill = {
  open: true,
  title: 'Kill The Long Haul S02E03?',
  consequences: ['Removes the download from qBittorrent'],
  choices: [
    { value: 'research', label: 'Blocklist and search again', consequences: ['Blocklists this release', 'Searches for another release now'] },
    { value: 'remove', label: 'Just remove' },
  ],
  confirmLabel: 'Kill download',
};

describe('ConfirmDialog', () => {
  it('is an alertdialog with consequences, choices and the undo statement; focus starts on Cancel', () => {
    const { getByRole, getAllByRole } = render(ConfirmDialog, kill);
    const dlg = getByRole('alertdialog', { name: 'Kill The Long Haul S02E03?' });
    expect(getAllByRole('listitem').map((li) => li.textContent)).toEqual([
      'Removes the download from qBittorrent', 'Blocklists this release', 'Searches for another release now',
    ]);
    expect((getByRole('radio', { name: 'Blocklist and search again' }) as HTMLInputElement).checked).toBe(true);
    expect(dlg.textContent).toContain('You can undo this');
    expect(document.activeElement).toBe(getByRole('button', { name: 'Cancel' }));
  });

  it('updates consequences with the choice and confirms with it', async () => {
    const onconfirm = vi.fn();
    const oncancel = vi.fn();
    const { getByRole, getAllByRole } = render(ConfirmDialog, { ...kill, onconfirm, oncancel });
    await fireEvent.click(getByRole('radio', { name: 'Just remove' }));
    expect(getAllByRole('listitem')).toHaveLength(1);
    await fireEvent.click(getByRole('button', { name: 'Kill download' }));
    expect(onconfirm).toHaveBeenCalledWith('remove');
    expect(oncancel).not.toHaveBeenCalled();
    expect(getByRole('alertdialog', { hidden: true }).hasAttribute('open')).toBe(false);
  });

  it('reports cancel from the Cancel button and Escape', async () => {
    const oncancel = vi.fn();
    const r = render(ConfirmDialog, { ...kill, oncancel });
    await fireEvent.click(r.getByRole('button', { name: 'Cancel' }));
    expect(oncancel).toHaveBeenCalledTimes(1);
    r.unmount();
    const r2 = render(ConfirmDialog, { ...kill, oncancel });
    await fireEvent(r2.getByRole('alertdialog'), new Event('cancel', { cancelable: true }));
    expect(oncancel).toHaveBeenCalledTimes(2);
  });

  it('states permanence, or a custom undo statement', () => {
    const p = render(ConfirmDialog, { open: true, title: 'Purge 3 files now?', confirmLabel: 'Purge', undoable: false });
    expect(p.getByRole('alertdialog').textContent).toContain('This is permanent and cannot be undone.');
    p.unmount();
    const c = render(ConfirmDialog, { open: true, title: 'Switch TV to live?', confirmLabel: 'Go live', variant: 'primary', undoable: 'Switch back to shadow at any time in Settings › Media kinds.' });
    expect(c.getByRole('alertdialog').textContent).toContain('Switch back to shadow at any time');
  });

  it('cannot be dismissed while the action runs', () => {
    const { getByRole } = render(ConfirmDialog, { ...kill, loading: true });
    expect((getByRole('button', { name: 'Cancel' }) as HTMLButtonElement).disabled).toBe(true);
    expect(getByRole('button', { name: 'Kill download' }).getAttribute('aria-busy')).toBe('true');
  });
});
