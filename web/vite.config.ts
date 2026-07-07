import { defineConfig } from 'vite';
import { resolve } from 'path';

export default defineConfig({
  root: '.',
  build: {
    // Output into the Go server's static dir so it serves the built JS
    outDir: '../static',
    emptyOutDir: false,
    rollupOptions: {
      input: resolve(__dirname, 'src/main.ts'),
      output: {
        entryFileNames: 'dashboard.js',
        // Inline all dynamic imports into the main bundle — no chunk files
        // This avoids needing type="module" and hashed chunk filenames
        inlineDynamicImports: true,
        assetFileNames: '[name][extname]',
      },
    },
  },
  server: {
    // Proxy API calls to the Go server during development
    proxy: {
      '/api': 'http://localhost:8420',
    },
  },
});
