import { defineConfig } from 'vite';

// One classic-script bundle that runs from file:// on 2018 TVs (Tizen 4 = Chromium 56,
// webOS 4 = Chromium 53): ES2015 syntax, IIFE (no ES modules), relative paths.
// postbuild.mjs strips type="module"/crossorigin from index.html for the same reason.
export default defineConfig({
  base: './',
  build: {
    target: 'es2015',
    cssTarget: 'chrome53',
    outDir: 'dist',
    emptyOutDir: true,
    assetsInlineLimit: 0,
    modulePreload: false,
    sourcemap: false,
    rollupOptions: {
      output: {
        format: 'iife',
        inlineDynamicImports: true,
        entryFileNames: 'app.js',
        assetFileNames: 'assets/[name][extname]',
      },
    },
  },
  server: { host: true, port: 5173 },
});
