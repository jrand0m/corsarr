import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import IndexersView from './IndexersView.svelte';
import { definitionCatalogue, failedTest, indexerList, indexerManifests, passedTest } from '../../../lib/storybook/indexer-fixtures';

function setup(over: Record<string, unknown> = {}) {
  const props = {
    rows: indexerList, manifests: indexerManifests, definitions: definitionCatalogue, directory: '/data/plugins/definitions',
    torznab: { enabled: true, all: '/torznab/all/api' }, origin: 'http://box:8989',
    onadd: vi.fn(async () => {}), onedit: vi.fn(async () => {}), ontest: vi.fn(async () => passedTest), ondelete: vi.fn(async () => {}),
    onreload: vi.fn(async () => {}), ontorznab: vi.fn(async () => {}), ...over,
  };
  render(IndexersView, { props });
  return props;
}

describe('IndexersView', () => {
  it('lists config and UI indexers with health and the shared addresses', () => {
    setup();
    expect(screen.getAllByText('Dock Feeds').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Action needed').length).toBeGreaterThan(0);
    expect(screen.getByText('http://box:8989/torznab/all/api')).toBeTruthy();
    expect(screen.queryByText('http://box:8989/torznab/tpb/api')).toBeNull(); // not running
  });

  it('switches an added indexer off and keeps config indexers locked', async () => {
    const p = setup();
    const tpb = screen.getByRole('switch', { name: 'Enable The Pirate Bay' });
    await fireEvent.click(tpb);
    await waitFor(() => expect(p.onedit).toHaveBeenCalledWith('tpb', { enabled: true }));
    expect((screen.getByRole('switch', { name: 'Enable Dock Feeds' }) as HTMLButtonElement).disabled).toBe(true);
  });

  it('shows why a new indexer was not saved', async () => {
    const onadd = vi.fn(async () => ({ test: failedTest }));
    setup({ onadd, initialAdd: 'harbourmusic' });
    await fireEvent.input(screen.getByLabelText(/^Key/), { target: { value: 'harbour2' } });
    await fireEvent.input(screen.getByLabelText(/Session cookie/), { target: { value: 'sid=1' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Test and save' }));
    await waitFor(() => expect(onadd).toHaveBeenCalled());
    expect(onadd.mock.calls[0]).toEqual([{ key: 'harbour2', plugin: 'harbourmusic', name: undefined, settings: { cookie: 'sid=1' } }]);
    await waitFor(() => expect(screen.getAllByText(/no longer accepts the session cookie/).length).toBeGreaterThan(0));
  });

  it('lists definition problems and turns the endpoint off', async () => {
    const p = setup();
    expect(screen.getByText('One definition file has problems')).toBeTruthy();
    await fireEvent.click(screen.getByRole('switch', { name: 'Torznab endpoint' }));
    await waitFor(() => expect(p.ontorznab).toHaveBeenCalledWith(false));
    await fireEvent.click(screen.getByRole('button', { name: 'Reload definitions' }));
    await waitFor(() => expect(p.onreload).toHaveBeenCalled());
  });
});
