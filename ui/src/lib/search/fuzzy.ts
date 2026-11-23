// Fuzzy matching for the command palette. Pure and allocation-light: the palette
// matches a few hundred commands and loaded items per keystroke.
// Every query character must appear in order; contiguous runs, word starts and a prefix
// match score higher, gaps and long texts score lower.

export interface FuzzyMatch {
  score: number;
  /** Matched character positions in `text` (for highlighting). */
  indices: number[];
}

const boundary = /[\s\-_./:·(]/;

export function fuzzyMatch(query: string, text: string): FuzzyMatch | null {
  const q = query.trim().toLowerCase();
  if (!q) return { score: 0, indices: [] };
  const t = text.toLowerCase();
  const indices: number[] = [];
  let score = 0;
  let from = 0;
  let prev = -2;
  for (const ch of q) {
    if (ch === ' ') continue;
    const at = t.indexOf(ch, from);
    if (at < 0) return null;
    let s = 1;
    if (at === prev + 1) s += 5;
    if (at === 0 || boundary.test(t[at - 1])) s += 8;
    s -= Math.min(3, (at - from) * 0.1);
    score += s;
    indices.push(at);
    prev = at;
    from = at + 1;
  }
  if (t.startsWith(q)) score += 10;
  score -= t.length * 0.01;
  return { score, indices };
}

/** Splits `text` into plain and matched runs for rendering with <mark>. */
export function highlight(text: string, indices: number[]): { text: string; hit: boolean }[] {
  const set = new Set(indices);
  const runs: { text: string; hit: boolean }[] = [];
  for (let i = 0; i < text.length; i++) {
    const hit = set.has(i);
    const last = runs[runs.length - 1];
    if (last && last.hit === hit) last.text += text[i];
    else runs.push({ text: text[i], hit });
  }
  return runs;
}
