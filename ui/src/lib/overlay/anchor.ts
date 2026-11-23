// Placement of a fixed-position popover next to its trigger (ActionMenu, EvidencePanel).
// Prefers below; flips above when there is no room below and room above. Aligns the panel's
// end (or start) edge with the trigger's and keeps an 8 px margin from the viewport edges.

export interface Rect { top: number; bottom: number; left: number; right: number }
export interface Size { width: number; height: number }

export function placeNear(
  trigger: Rect,
  panel: Size,
  viewport: Size,
  align: 'start' | 'end' = 'end',
  gap = 4,
): { top: number; left: number } {
  const margin = 8;
  const below = trigger.bottom + gap + panel.height <= viewport.height || trigger.top < panel.height + gap;
  const wanted = align === 'end' ? trigger.right - panel.width : trigger.left;
  return {
    top: below ? trigger.bottom + gap : trigger.top - gap - panel.height,
    left: Math.max(margin, Math.min(wanted, viewport.width - panel.width - margin)),
  };
}
