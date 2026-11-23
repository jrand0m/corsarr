// Pure helpers that turn a server-rendered widget into one self-contained HTML file: component
// CSS from the render head, only the design tokens that CSS uses (both themes), base.css, an
// optional hand-written script. No DOM, no I/O: build.mjs reads and writes the files.

/** Contents of every <style> element in a render head. */
export function extractStyles(head: string): string {
  return [...head.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]).join('\n');
}

/** Drops HTML comments (Svelte hydration markers such as <!--[--> and <!--]-->). */
export function stripComments(html: string): string {
  return html.replace(/<!--[\s\S]*?-->/g, '');
}

/** Reasonable CSS minification: comments, whitespace, spaces around punctuation, last semicolons. */
export function minifyCss(css: string): string {
  return css
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/\s+/g, ' ')
    .replace(/\s*([{};,>])\s*/g, '$1')
    .replace(/:\s+/g, ':')
    .replace(/;}/g, '}')
    .trim();
}

/** Custom properties declared in the first `:root` block of tokens.css, in source order. */
export function parseTokens(tokensCss: string): Map<string, string> {
  const css = tokensCss.replace(/\/\*[\s\S]*?\*\//g, '');
  const start = css.indexOf('{', css.indexOf(':root'));
  const end = css.indexOf('}', start);
  const tokens = new Map<string, string>();
  for (const decl of css.slice(start + 1, end).split(';')) {
    const m = decl.match(/^\s*(--[\w-]+)\s*:\s*([\s\S]+?)\s*$/);
    if (m) tokens.set(m[1], m[2].replace(/\s+/g, ' '));
  }
  return tokens;
}

/** Tokens the CSS reads, following tokens that read other tokens. */
export function usedTokens(css: string, tokens: Map<string, string>): Set<string> {
  const used = new Set<string>();
  const visit = (text: string) => {
    for (const [, name] of text.matchAll(/var\(\s*(--[\w-]+)/g)) {
      if (used.has(name) || !tokens.has(name)) continue;
      used.add(name);
      visit(tokens.get(name)!);
    }
  };
  visit(css);
  return used;
}

/** Resolves every light-dark(a, b) in a value to its light and its dark form. */
export function splitLightDark(value: string): [light: string, dark: string] {
  const at = value.indexOf('light-dark(');
  if (at < 0) return [value, value];
  let depth = 0;
  let comma = -1;
  let i = at + 'light-dark('.length;
  for (; i < value.length; i++) {
    const c = value[i];
    if (c === '(') depth++;
    else if (c === ')') {
      if (depth === 0) break;
      depth--;
    } else if (c === ',' && depth === 0 && comma < 0) comma = i;
  }
  const a = value.slice(at + 'light-dark('.length, comma).trim();
  const b = value.slice(comma + 1, i).trim();
  const [restLight, restDark] = splitLightDark(value.slice(i + 1));
  const before = value.slice(0, at);
  return [before + a + restLight, before + b + restDark];
}

/**
 * Token declarations for the used set. Light values on :root; dark values when the OS prefers
 * dark (unless the page sets data-theme="light") and when the page sets data-theme="dark".
 * light-dark() is expanded so the file works in webviews that predate it.
 */
export function tokenCss(tokens: Map<string, string>, used: Set<string>): string {
  const light: string[] = [];
  const dark: string[] = [];
  for (const [name, value] of tokens) {
    if (!used.has(name)) continue;
    const [l, d] = splitLightDark(value);
    light.push(`${name}:${l}`);
    if (d !== l) dark.push(`${name}:${d}`);
  }
  const darkBlock = ['color-scheme:dark', ...dark].join(';');
  return (
    `:root{${['color-scheme:light dark', ...light].join(';')}}` +
    `:root[data-theme=light]{color-scheme:light}` +
    `@media (prefers-color-scheme:dark){:root:not([data-theme=light]){${darkBlock}}}` +
    `:root[data-theme=dark]{${darkBlock}}`
  );
}

export interface PageParts {
  title: string;
  /** Component and base CSS, unminified, reading tokens through var(). */
  css: string;
  tokensCss: string;
  body: string;
  /** Hand-written page script, already minimal; omitted when empty. */
  script?: string;
}

/** Splits on commas outside parentheses (selector lists with :is(), :not(), :has()). */
function splitTop(text: string): string[] {
  const parts: string[] = [];
  let depth = 0;
  let from = 0;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (c === '(') depth++;
    else if (c === ')') depth--;
    else if (c === ',' && depth === 0) {
      parts.push(text.slice(from, i));
      from = i + 1;
    }
  }
  parts.push(text.slice(from));
  return parts;
}

/** Index of the brace closing the block that opens at `open`. */
function closing(css: string, open: number): number {
  let depth = 0;
  for (let i = open; i < css.length; i++) {
    if (css[i] === '{') depth++;
    else if (css[i] === '}' && --depth === 0) return i;
  }
  return css.length - 1;
}

/** Every class name used in the markup. */
export function htmlClasses(html: string): Set<string> {
  const set = new Set<string>();
  for (const [, list] of html.matchAll(/\sclass="([^"]*)"/g)) for (const c of list.split(/\s+/)) if (c) set.add(c);
  return set;
}

const usesOnly = (classes: Set<string>) => (selector: string) =>
  [...selector.replace(/:not\([^)]*\)/g, '').matchAll(/\.(-?[_a-zA-Z][\w-]*)/g)].every(([, c]) => classes.has(c));

/**
 * Drops selectors (from minified CSS) naming a class the markup never uses: component styles
 * for variants, tones and sizes this render does not show. Classes inside :not() do not count.
 */
export function pruneCss(css: string, classes: Set<string>): string {
  let out = '';
  let i = 0;
  while (i < css.length) {
    const open = css.indexOf('{', i);
    if (open < 0) break;
    const end = closing(css, open);
    const prelude = css.slice(i, open).trim();
    const block = css.slice(open + 1, end);
    i = end + 1;
    if (/^@(media|supports|container|layer)/.test(prelude)) {
      const inner = pruneCss(block, classes);
      if (inner) out += `${prelude}{${inner}}`;
    } else if (prelude.startsWith('@')) {
      out += `${prelude}{${block}}`;
    } else {
      const keep = splitTop(prelude).filter(usesOnly(classes));
      if (keep.length) out += `${keep.join(',')}{${block}}`;
    }
  }
  return out;
}

/** Replaces Svelte's scoping hashes (svelte-1gj33hp) with short names (_0) in markup and CSS. */
export function shortenScopes(html: string, css: string): [html: string, css: string] {
  const names = new Map<string, string>();
  const short = (h: string) => {
    if (!names.has(h)) names.set(h, `_${names.size.toString(36)}`);
    return names.get(h)!;
  };
  const re = /svelte-[a-z0-9]+/g;
  return [html.replace(re, short), css.replace(re, short)];
}

/** One self-contained HTML document. */
export function buildPage({ title, css, tokensCss, body, script }: PageParts): string {
  const tokens = parseTokens(tokensCss);
  const [html, scoped] = shortenScopes(stripComments(body).replace(/>\s+</g, '> <').trim(), minifyCss(css));
  const pruned = pruneCss(scoped, htmlClasses(html));
  const style = tokenCss(tokens, usedTokens(pruned, tokens)) + pruned;
  return (
    '<!doctype html><html lang="en"><head><meta charset="utf-8">' +
    '<meta name="viewport" content="width=device-width,initial-scale=1">' +
    `<meta name="color-scheme" content="light dark"><title>${title}</title>` +
    `<style>${style}</style></head><body><main>${html}</main>` +
    (script ? `<script>${script.trim()}</script>` : '') +
    '</body></html>\n'
  );
}

/** Adds data-left (seconds from `now`) to every countdown <time> so the page script can tick it. */
export function markCountdowns(html: string, now: number): string {
  return html.replace(/<time datetime="([^"]+)"([^>]*)>in /g, (_m, iso: string, rest: string) => {
    const left = Math.round((Date.parse(iso) - now) / 1000);
    return `<time datetime="${iso}" data-left="${left}"${rest}>in `;
  });
}
