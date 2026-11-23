import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import ActionMenu from './ActionMenu.svelte';

const items = [
  { id: 'search', label: 'Search now', group: 'a' },
  { id: 'off', label: 'Refresh', group: 'a', disabled: true },
  { id: 'delete', label: 'Delete…', group: 'b', danger: true },
];

describe('ActionMenu', () => {
  it('names the trigger, links it to a popover and separates groups', () => {
    const { getByRole, container } = render(ActionMenu, { label: 'More actions for Dockside', items });
    const trigger = getByRole('button', { name: 'More actions for Dockside' });
    const panel = container.ownerDocument.getElementById(trigger.getAttribute('popovertarget')!)!;
    expect(panel.getAttribute('popover')).toBe('auto');
    expect(trigger.getAttribute('aria-expanded')).toBe('false');
    expect(panel.querySelectorAll('[role="separator"]').length).toBe(1);
  });

  it('selects an item and skips disabled ones with arrows', async () => {
    const onselect = vi.fn();
    const { getByText, container } = render(ActionMenu, { label: 'Actions', items, onselect });
    (container.ownerDocument.querySelector('.panel') as HTMLElement).showPopover();
    const search = getByText('Search now').closest('button')!;
    expect(document.activeElement).toBe(search);
    await fireEvent.keyDown(container.ownerDocument.querySelector('.panel')!, { key: 'ArrowDown' });
    expect(document.activeElement).toBe(getByText('Delete…').closest('button'));
    await fireEvent.click(getByText('Delete…'));
    expect(onselect).toHaveBeenCalledWith('delete');
  });
});
