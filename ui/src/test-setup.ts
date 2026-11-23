// vitest setup: unmount between tests and provide browser APIs jsdom lacks.
import { afterEach } from 'vitest';
import { cleanup } from '@testing-library/svelte';

afterEach(cleanup);

if (typeof window !== 'undefined' && !window.matchMedia) {
  window.matchMedia = (query: string) =>
    ({
      matches: false, media: query, onchange: null,
      addListener() {}, removeListener() {}, addEventListener() {}, removeEventListener() {},
      dispatchEvent: () => false,
    }) as MediaQueryList;
}

// jsdom has no layout: ResizeObserver (bind:clientWidth) never fires, sizes stay 0.
if (typeof window !== 'undefined' && !('ResizeObserver' in window)) {
  (window as unknown as { ResizeObserver: unknown }).ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
}

// jsdom hides [popover] but lacks the Popover API: show by inline display, fire toggle events.
if (typeof HTMLElement !== 'undefined' && !HTMLElement.prototype.showPopover) {
  const toggle = (el: HTMLElement, open: boolean) => {
    el.style.display = open ? 'block' : '';
    el.dataset.popoverOpen = open ? 'true' : '';
    const e = new Event('toggle');
    Object.assign(e, { newState: open ? 'open' : 'closed', oldState: open ? 'closed' : 'open' });
    el.dispatchEvent(e);
  };
  HTMLElement.prototype.showPopover = function () { toggle(this, true); };
  HTMLElement.prototype.hidePopover = function () { toggle(this, false); };
  HTMLElement.prototype.togglePopover = function () { const open = this.dataset.popoverOpen !== 'true'; toggle(this, open); return open; };
}

// jsdom has <dialog> without showModal/close: toggle the open attribute and fire `close`.
// Escape is simulated in tests by dispatching `cancel`, as the browser does.
if (typeof HTMLDialogElement !== 'undefined' && !HTMLDialogElement.prototype.showModal) {
  const proto = HTMLDialogElement.prototype;
  if (!('open' in proto)) {
    Object.defineProperty(proto, 'open', {
      get(this: HTMLDialogElement) { return this.hasAttribute('open'); },
      set(this: HTMLDialogElement, v: boolean) { this.toggleAttribute('open', v); },
    });
  }
  proto.showModal = function () { this.setAttribute('open', ''); };
  proto.show = function () { this.setAttribute('open', ''); };
  proto.close = function (value?: string) {
    if (!this.hasAttribute('open')) return;
    this.removeAttribute('open');
    if (value !== undefined) this.returnValue = value;
    this.dispatchEvent(new Event('close'));
  };
}
