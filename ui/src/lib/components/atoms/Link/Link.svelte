<!--
  Link: navigation. `quiet` for links inside tables and lists (ink colour, underline on hover),
  `external` opens a new tab, adds the external icon and says so to screen readers.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAnchorAttributes } from 'svelte/elements';
  import Icon from '../Icon/Icon.svelte';

  interface Props extends HTMLAnchorAttributes {
    href: string;
    variant?: 'default' | 'quiet';
    external?: boolean;
    children: Snippet;
  }
  let { href, variant = 'default', external = false, class: cls, children, ...rest }: Props = $props();
</script>

<a
  {href}
  class={['link', variant, cls]}
  target={external ? '_blank' : undefined}
  rel={external ? 'noopener noreferrer' : undefined}
  {...rest}
>{@render children()}{#if external}<Icon name="external" size="sm" /><span class="visually-hidden"> (opens in a new tab)</span>{/if}</a>

<style>
  .link {
    color: var(--accent-ink);
    text-decoration: underline;
    text-decoration-thickness: 1px;
    text-underline-offset: 0.18em;
    border-radius: var(--radius-xs);
  }
  .link :global(svg) { display: inline-block; vertical-align: -0.1em; margin-left: 0.2em; }
  .link:hover { text-decoration-thickness: 2px; }
  .quiet { color: inherit; text-decoration-color: transparent; }
  .quiet:hover { color: var(--accent-ink); text-decoration-color: currentColor; }
</style>
