import { describe, expect, it } from 'vitest';
import { artSrc } from './art';

describe('artSrc', () => {
  it('asks the cache for twice the drawn width', () => {
    expect(artSrc('api/v1/images/abc', 40)).toBe('api/v1/images/abc?w=80');
    expect(artSrc('api/v1/images/abc', 120.4)).toBe('api/v1/images/abc?w=241');
  });
  it('leaves other addresses and missing artwork alone', () => {
    expect(artSrc(undefined, 40)).toBeNull();
    expect(artSrc('', 40)).toBeNull();
    expect(artSrc(null, 40)).toBeNull();
    expect(artSrc('https://example.test/p.jpg', 40)).toBe('https://example.test/p.jpg');
    expect(artSrc('api/v1/images/abc?w=10', 40)).toBe('api/v1/images/abc?w=10');
    expect(artSrc('api/v1/images/abc', 0)).toBe('api/v1/images/abc');
  });
});
