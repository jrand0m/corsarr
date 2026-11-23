import { describe, it, expect } from 'vitest';
import { placeNear } from './anchor';
import { focusables, trapTab, initialFocus } from './focus';

describe('placeNear', () => {
  const vp = { width: 1000, height: 800 };
  it('places below, end-aligned', () => {
    expect(placeNear({ top: 100, bottom: 130, left: 400, right: 500 }, { width: 200, height: 150 }, vp)).toEqual({ top: 134, left: 300 });
  });
  it('flips above when there is no room below', () => {
    expect(placeNear({ top: 700, bottom: 730, left: 400, right: 500 }, { width: 200, height: 150 }, vp).top).toBe(546);
  });
  it('start-aligns and clamps to the viewport margin', () => {
    expect(placeNear({ top: 0, bottom: 30, left: 900, right: 990 }, { width: 200, height: 100 }, vp, 'start').left).toBe(792);
    expect(placeNear({ top: 0, bottom: 30, left: 0, right: 40 }, { width: 200, height: 100 }, vp).left).toBe(8);
  });
});

describe('focus helpers', () => {
  function setup() {
    document.body.innerHTML = `
      <div id="root">
        <button data-overlay-close>Close</button>
        <div hidden><button>Hidden</button></div>
        <div popover><button>In closed popover</button></div>
        <input id="field" />
        <button id="last">Save</button>
      </div>`;
    return document.getElementById('root')!;
  }

  it('lists tabbables, skipping hidden subtrees and closed popovers', () => {
    expect(focusables(setup()).map((el) => el.textContent || el.id)).toEqual(['Close', 'field', 'Save']);
  });

  it('wraps Tab at both ends', () => {
    const root = setup();
    document.getElementById('last')!.focus();
    const fwd = new KeyboardEvent('keydown', { key: 'Tab', cancelable: true });
    trapTab(fwd, root);
    expect(fwd.defaultPrevented).toBe(true);
    expect(document.activeElement?.textContent).toBe('Close');
    const back = new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, cancelable: true });
    trapTab(back, root);
    expect(document.activeElement?.id).toBe('last');
  });

  it('prefers a marked element, then the first field', () => {
    const root = setup();
    expect(initialFocus(root).id).toBe('field');
    document.getElementById('last')!.setAttribute('data-autofocus', '');
    expect(initialFocus(root).id).toBe('last');
  });
});
