import { describe, expect, it } from 'vitest';
import { readNdjson } from './ndjson';

function streamOf(chunks: (string | Uint8Array)[], onCancel?: () => void): ReadableStream<Uint8Array> {
  const enc = new TextEncoder();
  let i = 0;
  return new ReadableStream<Uint8Array>({
    pull(c) {
      if (i >= chunks.length) { c.close(); return; }
      const x = chunks[i++];
      c.enqueue(typeof x === 'string' ? enc.encode(x) : x);
    },
    cancel() { onCancel?.(); },
  });
}

async function collect<T>(g: AsyncGenerator<T>): Promise<T[]> {
  const out: T[] = [];
  for await (const x of g) out.push(x);
  return out;
}

describe('readNdjson', () => {
  it('yields one value per line across arbitrary chunk boundaries', async () => {
    expect(await collect(readNdjson(streamOf(['{"a":', '1}\n{"b"', ':2}\n', '\n{"c":3}\n'])))).toEqual([{ a: 1 }, { b: 2 }, { c: 3 }]);
  });

  it('keeps multi-byte characters split between chunks intact', async () => {
    const bytes = new TextEncoder().encode('{"t":"Amélie ☂"}\n');
    const cut = bytes.indexOf(0xc3) + 1; // inside "é"
    expect(await collect(readNdjson(streamOf([bytes.slice(0, cut), bytes.slice(cut)])))).toEqual([{ t: 'Amélie ☂' }]);
  });

  it('reads a last line without a trailing newline', async () => {
    expect(await collect(readNdjson(streamOf(['{"x":1}\n{"x":2}'])))).toEqual([{ x: 1 }, { x: 2 }]);
  });

  it('cancels the body when the reader stops early', async () => {
    let cancelled = false;
    for await (const v of readNdjson(streamOf(['{"n":1}\n', '{"n":2}\n'], () => (cancelled = true)))) {
      expect(v).toEqual({ n: 1 });
      break;
    }
    expect(cancelled).toBe(true);
  });

  it('throws on a malformed line', async () => {
    await expect(collect(readNdjson(streamOf(['{"ok":1}\nnot json\n'])))).rejects.toThrow();
  });
});
