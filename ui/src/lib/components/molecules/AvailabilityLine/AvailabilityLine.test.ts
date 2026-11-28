import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import AvailabilityLine from './AvailabilityLine.svelte';

describe('AvailabilityLine', () => {
  it('names the status in words with the evidence given', () => {
    const { container } = render(AvailabilityLine, { status: 'may_be_incomplete', protocol: 'torrent', grabs: 210, files: 3, minimumRatio: 1 });
    expect(container.textContent).toContain('May be incomplete');
    expect(container.textContent).toContain('No one is seeding a full copy');
    expect(container.textContent).toContain('3 files · downloaded 210 times · seed to ratio 1.0');
  });

  it('leaves out evidence that was not reported and explains unknown usenet', () => {
    const { container } = render(AvailabilityLine, { status: 'unknown', protocol: 'usenet' });
    expect(container.textContent).toContain('The indexer does not report completeness');
    expect(container.querySelector('.evidence')).toBeNull();
  });
});
