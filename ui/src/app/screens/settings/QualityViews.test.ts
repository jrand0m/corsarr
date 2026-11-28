import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import CustomFormatsView from './CustomFormatsView.svelte';
import ProfilesView from './ProfilesView.svelte';
import { formatRows, formatTest, profileRows, qualityChoices } from '../../../lib/storybook/quality-fixtures';

describe('CustomFormatsView', () => {
  const setup = (over: Record<string, unknown> = {}) => {
    const props = { rows: formatRows, kinds: qualityChoices.conditionKinds, onsave: vi.fn(async () => {}), ondelete: vi.fn(async () => {}),
      ontest: vi.fn(async (t: string) => formatTest(t)), ...over };
    render(CustomFormatsView, { props });
    return props;
  };

  it('lists formats with what they test and who uses them', () => {
    setup();
    expect(screen.getAllByText('Audio - PS5 Safe (AC3/EAC3/AAC)').length).toBeGreaterThan(0);
    expect(screen.getAllByText(/Audio codec is AAC, AC3 \(Dolby Digital\) or EAC3/).length).toBeGreaterThan(0);
    expect(screen.getAllByText('2 profiles').length).toBeGreaterThan(0);
  });

  it('saves a new format and shows server errors on the condition', async () => {
    const onsave = vi.fn(async () => ({ fields: {}, conditions: [{ pattern: 'not a valid regular expression' }] }));
    setup({ onsave, initialEdit: '' });
    await fireEvent.input(screen.getByLabelText(/^Name/), { target: { value: 'Broken' } });
    await fireEvent.input(screen.getByLabelText(/^Pattern/), { target: { value: '(dts' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Save format' }));
    await waitFor(() => expect(onsave).toHaveBeenCalledWith(expect.objectContaining({ name: 'Broken', conditions: [{ kind: 'title', pattern: '(dts' }] }), true));
    expect(await screen.findByText('not a valid regular expression')).toBeTruthy();
  });

  it('opens config formats read-only', () => {
    setup({ initialEdit: 'nodirectplay' });
    expect(screen.getByText('Managed by config')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Save format' })).toBeNull();
  });
});

describe('ProfilesView', () => {
  const setup = (over: Record<string, unknown> = {}) => {
    const props = { rows: profileRows, formats: formatRows, choices: qualityChoices, onsave: vi.fn(async () => {}), ondelete: vi.fn(async () => {}),
      onduplicate: vi.fn(async () => ({ key: 'hd1080pcopy' })), ...over };
    render(ProfilesView, { props });
    return props;
  };

  it('lists built-in and custom profiles with what they serve and their use', () => {
    setup();
    expect(screen.getAllByText('High ~8GB/h - PS5').length).toBeGreaterThan(0);
    expect(screen.getAllByText('TV and movies').length).toBeGreaterThan(0);
    expect(screen.getAllByText('42').length).toBeGreaterThan(0);
  });

  it('edits scores and saves the document without row facts', async () => {
    const p = setup({ initialEdit: 'tv:highps5' });
    const score = await screen.findByLabelText(/^Audio - PS5 Safe/);
    await fireEvent.input(score, { target: { value: '150' } });
    await fireEvent.blur(score);
    await fireEvent.click(screen.getByRole('button', { name: 'Save profile' }));
    await waitFor(() => expect(p.onsave).toHaveBeenCalled());
    const [doc, isNew] = (p.onsave as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(isNew).toBe(false);
    expect(doc).not.toHaveProperty('items');
    expect(doc.formats.find((f: { format: string }) => f.format === 'ps5audio').score).toBe(150);
    expect(doc.formats.find((f: { format: string }) => f.format === 'nodirectplay').reject).toBe(true);
  });

  it('keeps built-ins read-only with a duplicate action', async () => {
    const p = setup({ initialEdit: 'tv:hd1080p' });
    expect(await screen.findByText('Built-in profiles stay as shipped')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Save profile' })).toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: 'Duplicate' }));
    await waitFor(() => expect(p.onduplicate).toHaveBeenCalled());
  });
});
