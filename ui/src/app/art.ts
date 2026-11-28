// Artwork addresses come from the API as cached image paths (api/v1/images/…). Asking for
// the width a screen draws at (twice, for sharp high-density screens) lets corsarr serve a
// smaller copy; any other address is returned unchanged.
const cached = /^api\/v1\/images\/[^?]+$/;

/** The artwork source for an image drawn `width` CSS pixels wide; null when there is none. */
export function artSrc(url: string | null | undefined, width: number): string | null {
  if (!url) return null;
  if (!cached.test(url) || !(width > 0)) return url;
  return `${url}?w=${Math.round(width * 2)}`;
}
