import { describe, it, expect } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import Artwork from './Artwork.svelte';

describe('Artwork', () => {
  it('lazy-loads with reserved dimensions', () => {
    const { getByRole } = render(Artwork, { src: '/a.jpg', title: 'Harbour Lights', kind: 'tv', width: 64 });
    const img = getByRole('img', { name: 'Harbour Lights' }) as HTMLImageElement;
    expect(img.getAttribute('loading')).toBe('lazy');
    expect(img.getAttribute('decoding')).toBe('async');
    expect(img.getAttribute('width')).toBe('64');
    expect(img.getAttribute('height')).toBe('96');
  });

  it('falls back to initials when the image fails or is missing', async () => {
    const { getByRole, container } = render(Artwork, { src: '/broken.jpg', title: 'The Long Meridian', kind: 'movie' });
    await fireEvent.error(container.querySelector('img')!);
    const fb = getByRole('img', { name: 'The Long Meridian' });
    expect(fb.tagName).toBe('SPAN');
    expect(fb.textContent).toContain('LM');
  });
});
