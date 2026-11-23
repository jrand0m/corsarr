<!--
  TemplateInput: a naming or message template ("{Series Title} - S{Season:00}E{Episode:00}")
  with a token picker that inserts at the caret and live examples rendered from sample data,
  so the owner sees the file name or message before saving. Unknown tokens are flagged.
  `{Token:00}` pads numbers to that many digits. Mono, because templates are data.
-->
<script lang="ts" module>
  export interface TemplateToken {
    /** Name inside the braces: "Series Title". */
    token: string;
    /** Group heading in the picker: "Series", "Episode", "Quality". */
    group?: string;
  }
  export interface TemplateSample { name: string; values: Record<string, string | number> }

  const TOKEN = /\{([^{}:]+)(?::([^{}]+))?\}/g;

  /** Render a template against values; returns the text and any unknown token names. */
  export function renderTemplate(template: string, values: Record<string, string | number>, known: Set<string>) {
    const unknown = new Set<string>();
    const text = template.replace(TOKEN, (whole, name: string, fmt?: string) => {
      const key = name.trim();
      if (!known.has(key)) { unknown.add(key); return whole; }
      const v = values[key];
      if (v === undefined || v === '') return '';
      if (fmt && /^0+$/.test(fmt) && /^\d+$/.test(String(v))) return String(v).padStart(fmt.length, '0');
      return String(v);
    });
    return { text, unknown: [...unknown] };
  }
</script>

<script lang="ts">
  import { tick } from 'svelte';
  import TextInput from '../../atoms/TextInput/TextInput.svelte';
  import Textarea from '../../atoms/Textarea/Textarea.svelte';
  import Icon from '../../atoms/Icon/Icon.svelte';

  interface Props {
    value?: string;
    tokens: TemplateToken[];
    samples: TemplateSample[];
    multiline?: boolean;
    label?: string;
    id?: string;
    describedby?: string;
    invalid?: boolean;
    readonly?: boolean;
    disabled?: boolean;
    required?: boolean;
    placeholder?: string;
    onchange?: (value: string) => void;
  }
  let {
    value = $bindable(''), tokens, samples, multiline = false, label, id, describedby, invalid = false,
    readonly = false, disabled = false, required = false, placeholder, onchange,
  }: Props = $props();

  const uid = $props.id();
  let wrap = $state<HTMLElement>();
  let caret = { start: -1, end: -1 };
  let pickerOpen = $state(false);

  const known = $derived(new Set(tokens.map((t) => t.token)));
  const groups = $derived.by(() => {
    const m = new Map<string, TemplateToken[]>();
    for (const t of tokens) m.set(t.group ?? 'Tokens', [...(m.get(t.group ?? 'Tokens') ?? []), t]);
    return [...m];
  });
  const rendered = $derived(samples.map((s) => ({ name: s.name, ...renderTemplate(value, s.values, known) })));
  const unknown = $derived([...new Set(rendered.flatMap((r) => r.unknown))]);

  const field = () => wrap?.querySelector<HTMLInputElement | HTMLTextAreaElement>('input, textarea');
  function remember() {
    const el = field();
    if (el) caret = { start: el.selectionStart ?? value.length, end: el.selectionEnd ?? value.length };
  }
  async function insert(t: TemplateToken) {
    const text = `{${t.token}}`;
    const start = caret.start < 0 ? value.length : caret.start;
    const end = caret.end < 0 ? value.length : caret.end;
    value = value.slice(0, start) + text + value.slice(end);
    caret = { start: start + text.length, end: start + text.length };
    onchange?.(value);
    await tick();
    const el = field();
    el?.focus();
    el?.setSelectionRange(caret.start, caret.end);
  }
</script>

<div class="tpl" bind:this={wrap}>
  {#if multiline}
    <Textarea
      {id} aria-label={id ? undefined : label} aria-describedby={[describedby, `${uid}-ex`].filter(Boolean).join(' ')}
      invalid={invalid || unknown.length > 0} {readonly} {disabled} {required} {placeholder} mono rows={4}
      bind:value spellcheck="false" onkeyup={remember} onclick={remember} onblur={remember} onchange={() => onchange?.(value)}
    />
  {:else}
    <TextInput
      {id} aria-label={id ? undefined : label} aria-describedby={[describedby, `${uid}-ex`].filter(Boolean).join(' ')}
      invalid={invalid || unknown.length > 0} {readonly} {disabled} {required} {placeholder} mono
      bind:value spellcheck="false" autocomplete="off" onkeyup={remember} onclick={remember} onblur={remember}
      onchange={() => onchange?.(value)}
    />
  {/if}

  {#if !readonly && !disabled}
    <div class="picker">
      <button type="button" class="disc" aria-expanded={pickerOpen} aria-controls="{uid}-tokens" onclick={() => (pickerOpen = !pickerOpen)}>
        <Icon name={pickerOpen ? 'chevron-down' : 'chevron-right'} size="sm" />Insert a token
      </button>
      <div class="groups" id="{uid}-tokens" hidden={!pickerOpen}>
        {#each groups as [group, list] (group)}
          <div class="group" role="group" aria-label="{group} tokens">
            <span class="gname" aria-hidden="true">{group}</span>
            {#each list as t (t.token)}
              <button type="button" class="tok" aria-label="Insert {`{${t.token}}`}" onpointerdown={(e) => e.preventDefault()} onclick={() => insert(t)}>
                {`{${t.token}}`}
              </button>
            {/each}
          </div>
        {/each}
      </div>
    </div>
  {/if}

  <div class="examples" id="{uid}-ex">
    {#if unknown.length}
      <p class="unknown"><Icon name="alert-octagon" size="sm" /><span>Unknown {unknown.length === 1 ? 'token' : 'tokens'}: {unknown.map((u) => `{${u}}`).join(', ')}</span></p>
    {/if}
    <dl>
      {#each rendered as r (r.name)}
        <div class="ex"><dt>{r.name}</dt><dd>{#if r.text.trim()}<code>{r.text}</code>{:else}<span class="none">Empty: add text or tokens</span>{/if}</dd></div>
      {/each}
    </dl>
  </div>
</div>

<style>
  .tpl { display: grid; gap: var(--space-2); min-width: 0; }
  .picker { display: grid; gap: var(--space-1-5); justify-items: start; }
  .disc { display: inline-flex; align-items: center; gap: var(--space-1); padding: var(--space-0-5) var(--space-1); margin-left: calc(var(--space-1) * -1); font-size: var(--text-sm); color: var(--accent-ink); background: none; border: 0; border-radius: var(--radius-sm); }
  .disc:hover { background: var(--surface-hover); }
  .groups { display: grid; gap: var(--space-2); width: 100%; padding: var(--space-2); background: var(--surface-sunken); border-radius: var(--radius-md); }
  .group { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-1); }
  .gname { width: 100%; font-size: var(--text-xs); color: var(--ink-muted); }
  .tok {
    min-height: var(--control-h-sm);
    padding: 0 var(--space-2);
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    color: var(--ink);
    background: var(--surface-raised);
    border: var(--border-w) solid var(--line-strong);
    border-radius: var(--radius-sm);
  }
  .tok:hover { background: var(--surface-hover); }
  .examples { display: grid; gap: var(--space-1-5); }
  dl { display: grid; gap: var(--space-1); margin: 0; }
  .ex { display: grid; grid-template-columns: minmax(6rem, 10rem) minmax(0, 1fr); gap: var(--space-1) var(--space-3); align-items: baseline; font-size: var(--text-sm); }
  dt { color: var(--ink-muted); }
  dd { margin: 0; min-width: 0; }
  code { font-size: var(--text-sm); overflow-wrap: anywhere; }
  .none { color: var(--ink-subtle); }
  .unknown { display: flex; gap: var(--space-1); align-items: flex-start; font-size: var(--text-sm); color: var(--tone-error-ink); max-width: none; }
  .unknown :global(svg) { margin-top: 0.125rem; }
  @media (width < 48rem) {
    .ex { grid-template-columns: minmax(0, 1fr); gap: 0; }
  }
  @media (width < 48rem), (pointer: coarse) {
    .tok, .disc { min-height: var(--target-min); }
  }
</style>
