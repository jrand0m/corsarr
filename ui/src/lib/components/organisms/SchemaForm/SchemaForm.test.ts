import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';
import SchemaForm from './SchemaForm.svelte';
import { everyFieldSchema, everyFieldValues, everyFieldMeta, categoryOptions, plexSchema, trackerSchema } from '../../../storybook/plugin-schema';

const base = { schema: everyFieldSchema, values: everyFieldValues, meta: everyFieldMeta, label: 'Harbour box settings', loadoptions: async () => categoryOptions };

describe('SchemaForm', () => {
  it('renders a control for every field type with zero plugin code', async () => {
    const { getByLabelText, getByRole, getByText } = render(SchemaForm, base);
    for (const name of ['Name', 'Address', 'Username', 'Parallel downloads', 'Completed folder']) expect(getByLabelText(name)).toBeTruthy();
    expect(getByRole('checkbox', { name: 'Add downloads paused' })).toBeTruthy();
    expect(getByRole('group', { name: /Media kinds/ })).toBeTruthy();
    expect(getByRole('group', { name: /Extra trackers/ })).toBeTruthy();
    expect(getByRole('group', { name: /Extra request headers/ })).toBeTruthy();
    expect(getByRole('group', { name: 'Connection' })).toBeTruthy();
    expect(getByText('From reference')).toBeTruthy();
    await waitFor(() => expect(getByRole('option', { name: 'movies' })).toBeTruthy());
  });

  it('shows a conditional field only when its parent has the value', async () => {
    const { getByRole, queryByLabelText } = render(SchemaForm, base);
    expect(queryByLabelText('Pause note')).toBeNull();
    await fireEvent.click(getByRole('checkbox', { name: 'Add downloads paused' }));
    expect(queryByLabelText('Pause note')).toBeTruthy();
  });

  it('tracks unsaved changes, saves only the changes and resets on cancel', async () => {
    const onsave = vi.fn();
    const { getByLabelText, getByRole, getByText } = render(SchemaForm, { ...base, onsave });
    const save = getByRole('button', { name: 'Save' }) as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    expect(getByText(/unsaved change|No changes|Saved/).textContent).toMatch('No changes');
    const name = getByLabelText('Name') as HTMLInputElement;
    await fireEvent.input(name, { target: { value: 'Dock 2' } });
    expect(getByText(/unsaved change|No changes|Saved/).textContent).toMatch('1 unsaved change');
    await fireEvent.click(getByRole('button', { name: 'Cancel' }));
    expect(name.value).toBe('Harbour box');
    await fireEvent.input(name, { target: { value: 'Dock 2' } });
    await fireEvent.click(save);
    await waitFor(() => expect(onsave).toHaveBeenCalledWith({ name: 'Dock 2' }));
    await waitFor(() => expect(getByText(/unsaved change|No changes|Saved/).textContent).toMatch('Saved'));
  });

  it('blocks Save with inline errors and a summary that jumps to the field', async () => {
    const onsave = vi.fn();
    const { getByRole, getByText, getAllByText, getByLabelText } = render(SchemaForm, { schema: everyFieldSchema, label: 'New client', onsave });
    await fireEvent.input(getByLabelText('Name'), { target: { value: 'Dock 3' } });
    await fireEvent.click(getByRole('button', { name: 'Save' }));
    expect(onsave).not.toHaveBeenCalled();
    expect(getByRole('alert').textContent).toMatch('3 settings need attention');
    expect(getAllByText('Address is required').length).toBeGreaterThan(0);
    await fireEvent.click(getByText('Address: Address is required'));
    expect(document.activeElement).toBe(getByLabelText('Address'));
  });

  it('shows server-side errors per field and clears one when its field changes', async () => {
    const onsave = vi.fn(async () => ({ url: 'Nothing answered at plex.lan' }));
    const { getByRole, getByLabelText, queryByText, getAllByText } = render(SchemaForm, {
      schema: plexSchema, values: { url: 'http://plex.lan' }, meta: { token: { isSet: true } }, label: 'Plex', onsave,
    });
    await fireEvent.click(getByRole('checkbox', { name: 'Refresh libraries after import' }));
    await fireEvent.click(getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(getAllByText('Nothing answered at plex.lan').length).toBeGreaterThan(0));
    await fireEvent.input(getByLabelText('Server address'), { target: { value: 'http://plex.lan:32400' } });
    expect(queryByText('Nothing answered at plex.lan')).toBeNull();
  });

  it('locks managed fields with their source key and marks drift', () => {
    const { getByLabelText, getByText, getAllByText } = render(SchemaForm, {
      ...base, meta: { ...everyFieldMeta, url: { managed: 'downloaders.harbourbox.url' }, slots: { drift: 'The client runs 5.' } },
    });
    expect((getByLabelText('Address') as HTMLInputElement).readOnly).toBe(true);
    expect(getByText(/at downloaders\.harbourbox\.url\./)).toBeTruthy();
    expect(getByText('The client runs 5.')).toBeTruthy();
    expect(getAllByText('Partly managed by config').length).toBe(1);
  });

  it('fills secret fields through a setup flow', async () => {
    const submit = vi.fn(async () => ({ ok: true as const }));
    const { getByRole, getByLabelText, getAllByText } = render(SchemaForm, { schema: trackerSchema, label: 'Tracker', setup: { submit } });
    await fireEvent.input(getByLabelText('Session cookie', { selector: 'textarea' }), { target: { value: 'uid=1; pass=abc' } });
    await fireEvent.click(getByRole('button', { name: 'Save cookie' }));
    expect(submit).toHaveBeenCalledWith(trackerSchema.setup![0], { cookie: 'uid=1; pass=abc' });
    await waitFor(() => expect(getAllByText('Connected').length).toBe(2));
    expect(getAllByText('Set').length).toBeGreaterThan(0);
  });
  it('reports the current values in their saved shape', async () => {
    const onvalues = vi.fn();
    const { getByLabelText } = render(SchemaForm, { ...base, onvalues });
    await fireEvent.input(getByLabelText('Address'), { target: { value: 'http://box:9091' } });
    await waitFor(() => expect(onvalues).toHaveBeenLastCalledWith(expect.objectContaining({ url: 'http://box:9091' })));
  });
});
