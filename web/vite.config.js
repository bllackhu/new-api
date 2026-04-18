/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import react from '@vitejs/plugin-react';
import { defineConfig, transformWithEsbuild } from 'vite';
import pkg from '@douyinfe/vite-plugin-semi';
import path from 'path';
import { codeInspectorPlugin } from 'code-inspector-plugin';
const { vitePluginSemi } = pkg;

// https://vitejs.dev/config/
export default defineConfig({
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      '@semi-global-css': path.resolve(
        __dirname,
        './node_modules/@douyinfe/semi-ui/dist/css/semi.css',
      ),
    },
  },
  plugins: [
    // Dev-only: adds work to the transform graph; disable in CI/Docker to save RAM on vite build.
    ...(process.env.DISABLE_CODE_INSPECTOR === 'true'
      ? []
      : [
          codeInspectorPlugin({
            bundler: 'vite',
          }),
        ]),
    {
      name: 'treat-js-files-as-jsx',
      async transform(code, id) {
        if (!/src\/.*\.js$/.test(id)) {
          return null;
        }

        // Use the exposed transform from vite, instead of directly
        // transforming with esbuild
        return transformWithEsbuild(code, id, {
          loader: 'jsx',
          jsx: 'automatic',
        });
      },
    },
    react(),
    vitePluginSemi({
      cssLayer: true,
    }),
  ],
  optimizeDeps: {
    esbuildOptions: {
      loader: {
        '.js': 'jsx',
        '.json': 'json',
      },
    },
  },
  build: {
    sourcemap: false,
    // Avoid gzip-size pass over every chunk (saves RAM on huge SPAs).
    reportCompressedSize: false,
    rollupOptions: {
      // Lower peak memory during large SPA builds (default Rollup parallelism can OOM small builders).
      maxParallelFileOps: Number(process.env.ROLLUP_MAX_PARALLEL || 2),
      output: {
        manualChunks: {
          'react-core': ['react', 'react-dom', 'react-router-dom'],
          'semi-ui': ['@douyinfe/semi-icons', '@douyinfe/semi-ui'],
          tools: ['axios', 'history', 'marked'],
          'react-components': [
            'react-dropzone',
            'react-fireworks',
            'react-telegram-login',
            'react-toastify',
            'react-turnstile',
          ],
          i18n: [
            'i18next',
            'react-i18next',
            'i18next-browser-languagedetector',
          ],
          visactor: [
            '@visactor/react-vchart',
            '@visactor/vchart',
            '@visactor/vchart-semi-theme',
          ],
          mermaid: ['mermaid'],
          antd: ['antd'],
          'lobe-icons': ['@lobehub/icons'],
          markdown: [
            'react-markdown',
            'remark-gfm',
            'remark-math',
            'remark-breaks',
            'rehype-highlight',
            'rehype-katex',
          ],
          katex: ['katex'],
          icons: ['react-icons', 'lucide-react'],
        },
      },
    },
  },
  server: {
    host: '0.0.0.0',
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:3001',
        changeOrigin: true,
      },
      '/mj': {
        target: 'http://127.0.0.1:3001',
        changeOrigin: true,
      },
      '/pg': {
        target: 'http://127.0.0.1:3001',
        changeOrigin: true,
      },
      '/v1': {
        target: 'http://127.0.0.1:3001',
        changeOrigin: true,
      },
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setupTests.js'],
    css: true,
  },
});
