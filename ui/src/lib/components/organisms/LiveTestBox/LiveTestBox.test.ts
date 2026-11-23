import { describe, it, expect, vi } from 'vitest';
import { render, fireEvent, waitFor } from '@testing-library/svelte';
import LiveTestBox, { type TestResult } from './LiveTestBox.svelte';

const result: TestResult = {
  verdict: { matched: true, text: 'Format applies' },
  score: 75,
  fields: [{ label: 'Resolution', value: '2160p' }, { label: 'Release group', value: null }],
  conditions: [
    { label: 'Resolution is 2160p', matched: true, required: true },
    { label: 'HDR present', matched: false, score: 50 },
  ],
};

describe('LiveTestBox', () => {
  it('tests after a pause in typing and shows fields and per-condition results', async () => {
    const test = vi.fn(async () => result);
    const { getByLabelText, getByText, getAllByText } = render(LiveTestBox, { label: 'Release title', test, delay: 10 });
    expect(getByText('Paste a title above to see how it is read.')).toBeTruthy();
    const input = getByLabelText('Release title');
    await fireEvent.input(input, { target: { value: 'Quay' } });
    await fireEvent.input(input, { target: { value: 'Quay.Lights.2160p' } });
    await waitFor(() => expect(getByText('Format applies')).toBeTruthy());
    expect(test).toHaveBeenCalledTimes(1);
    expect(test).toHaveBeenCalledWith('Quay.Lights.2160p');
    expect(getByText('1 of 2 conditions match')).toBeTruthy();
    expect(getByText('1 not recognised')).toBeTruthy();
    expect(getAllByText('Matches').length).toBe(1);
    expect(getAllByText('No match').length).toBe(1);
  });

  it('drops a stale answer when a newer test finished first', async () => {
    let slow!: (r: TestResult) => void;
    const test = vi.fn((s: string) => (s === 'old' ? new Promise<TestResult>((r) => (slow = r)) : Promise.resolve({ verdict: { matched: false, text: 'Newest' } })));
    const { getByLabelText, getByText, queryByText } = render(LiveTestBox, { label: 'Release title', test, live: false });
    const input = getByLabelText('Release title');
    await fireEvent.input(input, { target: { value: 'old' } });
    await fireEvent.keyDown(input, { key: 'Enter' });
    await fireEvent.input(input, { target: { value: 'new' } });
    await fireEvent.keyDown(input, { key: 'Enter' });
    await waitFor(() => expect(getByText('Newest')).toBeTruthy());
    slow({ verdict: { matched: true, text: 'Old answer' } });
    await Promise.resolve();
    expect(queryByText('Old answer')).toBeNull();
  });

  it('fills the input from a sample and reports a failure', async () => {
    const test = vi.fn(async () => { throw new Error('parser restarting'); });
    const { getByRole, getByText, getByLabelText } = render(LiveTestBox, { label: 'Release title', test, samples: ['The.Long.Haul.S02E03'] });
    await fireEvent.click(getByRole('button', { name: 'The.Long.Haul.S02E03' }));
    expect((getByLabelText('Release title') as HTMLTextAreaElement).value).toBe('The.Long.Haul.S02E03');
    await waitFor(() => expect(getByText("Couldn't test this: parser restarting")).toBeTruthy());
  });
});
