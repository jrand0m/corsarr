import type { Preview } from '@storybook/svelte-vite';
import '../src/lib/tokens/tokens.css';
import '../src/lib/tokens/base.css';

const preview: Preview = {
  parameters: {
    layout: 'padded',
    a11y: { test: 'error' },
    // Page stories pin these through `globals.viewport` (375 px phone, 1280 px desktop).
    viewport: {
      options: {
        phone: { name: 'Phone 375', styles: { width: '375px', height: '812px' }, type: 'mobile' },
        desktop: { name: 'Desktop 1280', styles: { width: '1280px', height: '800px' }, type: 'desktop' },
      },
    },
    options: { storySort: { order: ['Foundations', 'Atoms', 'Molecules', 'Organisms', 'Templates', 'Pages'] } },
  },
  globalTypes: {
    theme: {
      description: 'Colour theme',
      toolbar: { title: 'Theme', items: ['light', 'dark'], dynamicTitle: true },
    },
  },
  initialGlobals: { theme: 'light' },
  decorators: [
    (story, ctx) => {
      document.documentElement.dataset.theme = ctx.globals.theme;
      return story();
    },
  ],
};
export default preview;
