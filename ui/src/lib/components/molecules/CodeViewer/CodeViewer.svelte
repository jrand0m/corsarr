<!--
  CodeViewer: read-only YAML, env, JSON or a rendered message, with line numbers, wrap
  toggle, copy and download. Lightweight tinting only (comments quieter, keys medium weight):
  no highlighter dependency. The scroll region is focusable so keyboard users can scroll it.
-->
<script lang="ts" module>
  export type CodeLanguage = 'yaml' | 'env' | 'json' | 'text';
  export interface CodeToken { text: string; kind: 'plain' | 'comment' | 'key' | 'punct' }

  /** Split one line into display tokens. Deliberately small: keys and comments only. */
  export function tokenizeLine(line: string, language: CodeLanguage): CodeToken[] {
    if (language === 'text') return [{ text: line, kind: 'plain' }];
    const hash = language === 'json' ? -1 : line.search(/(^|\s)#/);
    const body = hash >= 0 ? line.slice(0, hash) : line;
    const comment = hash >= 0 ? line.slice(hash) : '';
    const out: CodeToken[] = [];
    const key = language === 'yaml' ? /^(\s*(?:- )?)([^\s:#][^:#]*?)(:)(\s|$)/
      : language === 'env' ? /^(\s*(?:export\s+)?)([A-Za-z_][A-Za-z0-9_]*)(=)()/
      : /^(\s*)("(?:[^"\\]|\\.)*")(\s*:)()/;
    const m = key.exec(body);
    if (m) {
      if (m[1]) out.push({ text: m[1], kind: 'plain' });
      out.push({ text: m[2], kind: 'key' }, { text: m[3], kind: 'punct' });
      const rest = body.slice(m[1].length + m[2].length + m[3].length);
      if (rest) out.push({ text: rest, kind: 'plain' });
    } else if (body) out.push({ text: body, kind: 'plain' });
    if (comment) out.push({ text: comment, kind: 'comment' });
    return out;
  }
</script>

<script lang="ts">
  import Button from '../../atoms/Button/Button.svelte';
  import IconButton from '../../atoms/IconButton/IconButton.svelte';

  interface Props {
    code: string;
    language?: CodeLanguage;
    /** Shown in the header and used for download: "corsarr.yaml". */
    filename?: string;
    /** Accessible name of the code region; defaults to the filename. */
    label?: string;
    lineNumbers?: boolean;
    wrap?: boolean;
    download?: boolean;
    /** Max height before the region scrolls, as a CSS length. */
    maxHeight?: string;
  }
  let {
    code, language = 'text', filename, label, lineNumbers = true, wrap = $bindable(false), download = true, maxHeight = '24rem',
  }: Props = $props();

  const lines = $derived(code.replace(/\n$/, '').split('\n'));
  const mime: Record<CodeLanguage, string> = { yaml: 'application/yaml', env: 'text/plain', json: 'application/json', text: 'text/plain' };

  let copied = $state(false);
  let timer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => () => clearTimeout(timer));
  async function copy() {
    try { await navigator.clipboard.writeText(code); } catch { return; }
    copied = true;
    clearTimeout(timer);
    timer = setTimeout(() => (copied = false), 2000);
  }

  function save() {
    const url = URL.createObjectURL(new Blob([code], { type: mime[language] }));
    const a = document.createElement('a');
    a.href = url;
    a.download = filename ?? `corsarr.${language === 'text' ? 'txt' : language}`;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 0);
  }
</script>

<figure class="cv">
  <figcaption class="bar">
    <span class="name">{filename ?? language.toUpperCase()}</span>
    <span class="meta num">{lines.length} {lines.length === 1 ? 'line' : 'lines'}</span>
    <span class="tools">
      <IconButton icon="list" label={wrap ? 'Do not wrap lines' : 'Wrap long lines'} size="sm" pressed={wrap} onclick={() => (wrap = !wrap)} />
      <Button size="sm" variant="ghost" icon={copied ? 'check' : 'copy'} onclick={copy}>{copied ? 'Copied' : 'Copy'}</Button>
      {#if download}<Button size="sm" variant="ghost" icon="download" onclick={save}>Download</Button>{/if}
    </span>
  </figcaption>
  <span class="visually-hidden" role="status">{copied ? 'Copied to clipboard' : ''}</span>
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <div class="scroll" style:max-height={maxHeight} tabindex="0" role="region" aria-label={label ?? filename ?? `${language} code`}>
    <pre class:wrap class:numbered={lineNumbers}><code>{#each lines as line, i (i)}<span class="ln">{#if lineNumbers}<span class="no" aria-hidden="true">{i + 1}</span>{/if}<span class="tx">{#each tokenizeLine(line, language) as t, j (j)}<span class={t.kind}>{t.text}</span>{/each}{'\n'}</span></span>{/each}</code></pre>
  </div>
</figure>

<style>
  .cv { display: grid; min-width: 0; margin: 0; background: var(--surface-sunken); border: var(--border-w) solid var(--line); border-radius: var(--radius-lg); overflow: hidden; }
  .bar { display: flex; align-items: center; gap: var(--space-2); min-height: var(--control-h-md); padding: 0 var(--space-1) 0 var(--space-3); background: var(--surface-raised); border-bottom: var(--border-w) solid var(--line); }
  .name { font-size: var(--text-sm); font-weight: var(--weight-medium); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .meta { font-size: var(--text-xs); color: var(--ink-subtle); white-space: nowrap; }
  .tools { display: flex; align-items: center; gap: var(--space-0-5); margin-left: auto; }
  .scroll { overflow: auto; overscroll-behavior: contain; }
  pre { margin: 0; padding: var(--space-2) 0; font-family: var(--font-mono); font-size: var(--text-sm); line-height: var(--leading-normal); color: var(--ink); tab-size: 2; }
  code { font-size: inherit; display: block; min-width: max-content; }
  .wrap code { min-width: 0; }
  .ln { display: flex; }
  .no { flex: none; width: 3.5ch; margin-right: var(--space-3); padding-right: var(--space-2); text-align: right; color: var(--ink-subtle); user-select: none; border-right: var(--border-w) solid var(--line); font-variant-numeric: tabular-nums; }
  .tx { padding: 0 var(--space-3); white-space: pre; }
  .numbered .tx { padding-left: 0; }
  .wrap .tx { white-space: pre-wrap; overflow-wrap: anywhere; min-width: 0; }
  .comment { color: var(--ink-subtle); font-style: italic; }
  .key { font-weight: var(--weight-semibold); }
  .punct { color: var(--ink-muted); }
  @media (width < 48rem) {
    .meta { display: none; }
    .tools :global(.btn .label) { display: none; }
  }
</style>
