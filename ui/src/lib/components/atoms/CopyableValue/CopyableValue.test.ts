import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import CopyableValue from './CopyableValue.svelte';

describe('CopyableValue', () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  beforeEach(() => {
    writeText.mockClear();
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
  });

  it('copies the value and confirms', async () => {
    const oncopy = vi.fn();
    const { getByRole, findByRole } = render(CopyableValue, { label: 'iCal URL', value: 'http://x/cal.ics', oncopy });
    await fireEvent.click(getByRole('button', { name: 'Copy iCal URL' }));
    expect(writeText).toHaveBeenCalledWith('http://x/cal.ics');
    expect(oncopy).toHaveBeenCalledWith('http://x/cal.ics');
    expect((await findByRole('status')).textContent).toBe('Copied');
  });

  it('masks a secret until shown, and still copies the real value', async () => {
    const { container, getByRole } = render(CopyableValue, { label: 'API key', value: 'abc123secret', secret: true });
    expect(container.textContent).not.toContain('abc123secret');
    await fireEvent.click(getByRole('button', { name: 'Show API key' }));
    expect(container.textContent).toContain('abc123secret');
    await fireEvent.click(getByRole('button', { name: 'Copy API key' }));
    expect(writeText).toHaveBeenCalledWith('abc123secret');
  });
});
