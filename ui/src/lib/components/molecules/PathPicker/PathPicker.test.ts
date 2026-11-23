import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import PathPicker from './PathPicker.svelte';

const locations = [
  { path: '/data/media/tv', label: 'TV library', free: 412e9, total: 2e12, access: 'writable' as const },
  { path: '/mnt/usb/tv', access: 'unreachable' as const, problem: 'not mounted' },
];

describe('PathPicker', () => {
  it('lists known locations with free space and access problems', () => {
    const { getByRole, getByText } = render(PathPicker, { label: 'Root folder', value: '/data/media/tv', locations });
    expect(getByRole('radiogroup', { name: 'Root folder' })).toBeTruthy();
    expect((getByRole('radio', { name: /\/data\/media\/tv/ }) as HTMLInputElement).checked).toBe(true);
    expect(getByRole('meter', { name: 'Space used on /data/media/tv' }).getAttribute('aria-valuetext')).toBe('412 GB free of 2.0 TB');
    expect(getByText(/Unreachable/)).toBeTruthy();
  });

  it('picks a location or a typed folder', async () => {
    const onchange = vi.fn();
    const { getByRole, getByLabelText } = render(PathPicker, { label: 'Root folder', value: '/data/media/tv', locations, onchange });
    await fireEvent.click(getByRole('radio', { name: /\/mnt\/usb\/tv/ }));
    expect(onchange).toHaveBeenLastCalledWith('/mnt/usb/tv');
    await fireEvent.input(getByLabelText('Another folder path'), { target: { value: '/srv/tv' } });
    expect(onchange).toHaveBeenLastCalledWith('/srv/tv');
    expect((getByRole('radio', { name: /Another folder/ }) as HTMLInputElement).checked).toBe(true);
  });

  it('is a plain path input without known locations', () => {
    const { getByRole } = render(PathPicker, { label: 'Download folder', value: '/downloads' });
    expect((getByRole('textbox', { name: 'Download folder' }) as HTMLInputElement).value).toBe('/downloads');
  });
});
