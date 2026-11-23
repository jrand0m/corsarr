// Builds the Telegram HTML widget templates: server-renders each widget
// with Svelte through Vite's module loader (component CSS injected into the render head), then
// writes one self-contained HTML file per widget to src/telegram/dist/.
// Run: npm run build-telegram
import { createServer } from 'vite';
import { svelte, vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

// Reproducible output: absolute times in tooltips render in UTC.
process.env.TZ = 'UTC';

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, '..', '..');
const out = join(here, 'dist');

const server = await createServer({
  root,
  configFile: false,
  logLevel: 'warn',
  appType: 'custom',
  server: { middlewareMode: true, hmr: false, ws: false },
  plugins: [svelte({ configFile: false, preprocess: vitePreprocess(), emitCss: false, compilerOptions: { css: 'injected' } })],
});

try {
  const mod = await server.ssrLoadModule('/src/telegram/entry.ts');
  const tokensCss = await readFile(join(root, 'src/lib/tokens/tokens.css'), 'utf8');
  const baseCss = await readFile(join(root, 'src/lib/tokens/base.css'), 'utf8');
  const script = await readFile(join(here, 'page-script.js'), 'utf8');
  await mkdir(out, { recursive: true });
  for (const w of mod.renderAll()) {
    const html = mod.buildPage({
      title: w.title,
      css: baseCss + '\n' + mod.extractStyles(w.head),
      tokensCss,
      body: mod.markCountdowns(w.body, w.now),
      script,
    });
    await writeFile(join(out, w.file), html);
    console.log(`${w.file}: ${Buffer.byteLength(html)} bytes`);
  }
} finally {
  await server.close();
}
