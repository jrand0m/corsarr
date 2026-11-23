import { describe, expect, it } from 'vitest';
import { render, screen, within } from '@testing-library/svelte';
import { readFileSync } from 'node:fs';
import VariantPickerWidget from './VariantPickerWidget.svelte';
import QueueListWidget from './QueueListWidget.svelte';
import { queueListSample, variantPickerSample } from './samples';
import {
  buildPage, htmlClasses, markCountdowns, minifyCss, parseTokens, pruneCss, shortenScopes, splitLightDark, stripComments, tokenCss, usedTokens,
} from './inline';

const read = (path: string) => readFileSync(new URL(path, import.meta.url), 'utf8');

describe('VariantPickerWidget', () => {
  it('is a plain form: radios with the default checked and named submit buttons', () => {
    const { container } = render(VariantPickerWidget, { data: variantPickerSample });
    const form = container.querySelector('form')!;
    expect(form.getAttribute('method')).toBe('post');
    expect(form.getAttribute('action')).toBe('/telegram/replace/d2');
    const radios = within(form).getAllByRole('radio') as HTMLInputElement[];
    expect(radios.map((r) => [r.name, r.value, r.checked])).toEqual([['variant', 'l1', true], ['variant', 'l2', false], ['variant', 'l3', false]]);
    expect(within(form).getByRole('button', { name: 'Use this release' })).toMatchObject({ type: 'submit', name: 'action', value: 'pick' });
    expect(within(form).getByRole('button', { name: 'Cancel' })).toMatchObject({ type: 'submit', name: 'action', value: 'cancel' });
  });

  it('names the download being replaced', () => {
    render(VariantPickerWidget, { data: variantPickerSample });
    expect(screen.getByRole('heading', { level: 2 }).textContent).toBe('Dockside · 2026');
    expect(screen.getByText(/^Now: 2160p WEB-DL/)).toBeTruthy();
  });
});

describe('QueueListWidget', () => {
  it('gives each unfinished download its own form posting id and action', () => {
    render(QueueListWidget, { data: queueListSample });
    const forms = screen.getAllByRole('form');
    expect(forms).toHaveLength(4);
    for (const form of forms) {
      expect(form.getAttribute('method')).toBe('post');
      expect(form.getAttribute('action')).toBe('/telegram/queue');
      expect(form.querySelector<HTMLInputElement>('input[type=hidden][name=id]')?.value).toMatch(/^d\d$/);
    }
    const first = screen.getByRole('form', { name: 'Actions for Dockside' });
    expect(within(first).getAllByRole('button').map((b) => [(b as HTMLButtonElement).name, (b as HTMLButtonElement).value, b.getAttribute('type')])).toEqual([
      ['action', 'pause', 'submit'], ['action', 'replace', 'submit'], ['action', 'replace-lower', 'submit'], ['action', 'kill', 'submit'],
    ]);
    // Visible "Lower…"; the hidden word completes the name (jsdom's accname drops the space).
    expect(first.querySelector('[value=replace-lower]')?.textContent?.trim()).toBe('Lower resolution…');
  });

  it('offers Resume on a paused download and no actions on a finished one', () => {
    render(QueueListWidget, { data: queueListSample });
    expect(within(screen.getByRole('form', { name: 'Actions for Harbour Lights' })).getByRole('button', { name: 'Resume' })).toBeTruthy();
    expect(screen.queryByRole('form', { name: 'Actions for 港の灯り' })).toBeNull();
    expect(screen.getByText('5 downloads · 2 need attention')).toBeTruthy();
  });

  it('limits the buttons to the actions offered, and says when the queue is empty', () => {
    const { unmount } = render(QueueListWidget, { data: { ...queueListSample, actions: ['pause', 'kill'] } });
    const form = screen.getByRole('form', { name: 'Actions for Dockside' });
    expect(within(form).getAllByRole('button').map((b) => (b as HTMLButtonElement).value)).toEqual(['pause', 'kill']);
    unmount();
    render(QueueListWidget, { data: { ...queueListSample, downloads: [] } });
    expect(screen.getByText(/Nothing downloading/)).toBeTruthy();
  });
});

describe('inline helpers', () => {
  it('splits light-dark() into both themes, nested values included', () => {
    expect(splitLightDark('light-dark(oklch(0.9 0 0), oklch(0.2 0 0))')).toEqual(['oklch(0.9 0 0)', 'oklch(0.2 0 0)']);
    expect(splitLightDark('0 1px 2px light-dark(oklch(0.2 0.01 60 / 0.1), oklch(0 0 0 / 0.4))')).toEqual([
      '0 1px 2px oklch(0.2 0.01 60 / 0.1)', '0 1px 2px oklch(0 0 0 / 0.4)',
    ]);
    expect(splitLightDark('0.75rem')).toEqual(['0.75rem', '0.75rem']);
  });

  it('keeps only the tokens the CSS reads, following references, with a dark block', () => {
    const tokens = parseTokens(':root,[data-theme]{ /* c */ --a: light-dark(red, blue); --b: var(--c); --c: 2px; --unused: 1px; }');
    const used = usedTokens('.x{color:var(--a);margin:var(--b)}', tokens);
    expect([...used].sort()).toEqual(['--a', '--b', '--c']);
    const css = tokenCss(tokens, used);
    expect(css).toContain(':root{color-scheme:light dark;--a:red;--b:var(--c);--c:2px}');
    expect(css).toContain('@media (prefers-color-scheme:dark){:root:not([data-theme=light]){color-scheme:dark;--a:blue}}');
    expect(css).toContain(':root[data-theme=dark]{color-scheme:dark;--a:blue}');
    expect(css).not.toContain('--unused');
  });

  it('prunes selectors for classes the markup never uses, inside media queries too', () => {
    const css = minifyCss('.a { color: red; }\n.b, .a:hover { color: blue; }\n@media (width < 48rem) { .b { x: 1 } }\n.a:not(.b) { y: 2 }\n@keyframes s { to { opacity: 0 } }');
    expect(pruneCss(css, htmlClasses('<p class="a"></p>'))).toBe('.a{color:red}.a:hover{color:blue}.a:not(.b){y:2}@keyframes s{to{opacity:0}}');
  });

  it('strips hydration comments, shortens scope classes and marks countdowns', () => {
    expect(stripComments('<!--[--><p>x</p><!--]--><!---->')).toBe('<p>x</p>');
    expect(shortenScopes('<p class="a svelte-1abc"></p><i class="svelte-2xyz"></i>', '.a.svelte-1abc{}.svelte-2xyz{}')).toEqual([
      '<p class="a _0"></p><i class="_1"></i>', '.a._0{}._1{}',
    ]);
    expect(markCountdowns('<time datetime="2026-11-22T14:05:42.000Z" class="_4">in 0:42</time>', Date.parse('2026-11-22T14:05:00Z'))).toBe(
      '<time datetime="2026-11-22T14:05:42.000Z" data-left="42" class="_4">in 0:42</time>',
    );
  });

  it('builds one self-contained document', () => {
    const html = buildPage({ title: 'T', css: '.a { color: var(--ink); }', tokensCss: ':root{--ink: light-dark(black, white);}', body: '<!--[--><p class="a">x</p><!--]-->' });
    expect(html).toMatch(/^<!doctype html>/);
    expect(html).toContain('<style>:root{color-scheme:light dark;--ink:black}');
    expect(html).toContain('<body><main><p class="a">x</p></main></body>');
    expect(html).not.toContain('<script');
  });
});

describe('built widget files', () => {
  it('keeps the page script under 1 KiB', () => {
    expect(new TextEncoder().encode(read('./page-script.js')).length).toBeLessThan(1024);
  });

  for (const file of ['variant-picker.html', 'queue-cards.html']) {
    it(`${file} is one small self-contained file of plain forms`, () => {
      const html = read(`./dist/${file}`);
      expect(new TextEncoder().encode(html).length).toBeLessThan(30 * 1024);
      expect(html).not.toMatch(/<!--|<link|<img|src=|url\(|https?:\/\//);
      expect(html.match(/<script>/g)?.length).toBe(1);
      expect(html).toMatch(/<form [^>]*method="post" action="\/telegram\//);
      expect(html).not.toContain('light-dark(');
    });
  }
});
