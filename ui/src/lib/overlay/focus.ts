// Focus helpers shared by the overlay organisms (Dialog, Drawer, CommandPalette).
// Native modal <dialog> already makes the page inert; these keep Tab cycling inside the
// overlay (instead of escaping to the browser chrome) and pick the first field to focus.

const focusable = [
  'a[href]', 'button:not(:disabled)', 'input:not(:disabled):not([type="hidden"])', 'select:not(:disabled)',
  'textarea:not(:disabled)', 'summary', '[tabindex]:not([tabindex="-1"])', '[contenteditable="true"]',
].join(',');

function popoverOpen(el: Element): boolean {
  try {
    return el.matches(':popover-open');
  } catch {
    return (el as HTMLElement).dataset.popoverOpen === 'true';
  }
}

/** Tabbable elements inside `root`, skipping hidden subtrees and closed popovers. */
export function focusables(root: HTMLElement): HTMLElement[] {
  return [...root.querySelectorAll<HTMLElement>(focusable)].filter((el) => {
    if (el.closest('[hidden], [inert]')) return false;
    const pop = el.closest('[popover]');
    return !pop || pop === root || popoverOpen(pop);
  });
}

/** Keydown handler: wraps Tab and Shift+Tab at the ends of `root`. */
export function trapTab(e: KeyboardEvent, root: HTMLElement): void {
  if (e.key !== 'Tab') return;
  const list = focusables(root);
  if (!list.length) {
    e.preventDefault();
    return;
  }
  const first = list[0];
  const last = list[list.length - 1];
  const active = document.activeElement;
  if (e.shiftKey && (active === first || !root.contains(active))) {
    e.preventDefault();
    last.focus();
  } else if (!e.shiftKey && (active === last || !root.contains(active))) {
    e.preventDefault();
    first.focus();
  }
}

/**
 * The element to focus when an overlay opens: `[data-autofocus]`, else the first field, else
 * the first tabbable that is not the close button, else the overlay itself.
 */
export function initialFocus(root: HTMLElement): HTMLElement {
  const marked = root.querySelector<HTMLElement>('[data-autofocus]');
  if (marked) return marked;
  const list = focusables(root);
  return (
    list.find((el) => el.matches('input, select, textarea')) ??
    list.find((el) => !el.closest('[data-overlay-close]')) ??
    list[0] ??
    root
  );
}
