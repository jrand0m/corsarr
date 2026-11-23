// NDJSON reader: one JSON value per line, yielded as soon as its line is complete.
// Works on any byte stream; chunk boundaries may fall anywhere, including inside a UTF-8 sequence.

export async function* readNdjson<T = unknown>(body: ReadableStream<Uint8Array>): AsyncGenerator<T> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buf = '';
  let finished = false;
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) { finished = true; break; }
      buf += decoder.decode(value, { stream: true });
      let nl: number;
      while ((nl = buf.indexOf('\n')) >= 0) {
        const line = buf.slice(0, nl).trim();
        buf = buf.slice(nl + 1);
        if (line) yield JSON.parse(line) as T;
      }
    }
    const last = (buf + decoder.decode()).trim();
    if (last) yield JSON.parse(last) as T;
  } finally {
    // Stopped early (the consumer broke out): cancel the body so the server stops streaming.
    if (!finished) await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}
