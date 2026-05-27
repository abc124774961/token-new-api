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
const cliPortIndex = process.argv.findIndex((arg) => arg === '--port');
const cliPort =
  cliPortIndex >= 0
    ? process.argv[cliPortIndex + 1]
    : process.argv
        .find((arg) => arg.startsWith('--port='))
        ?.slice('--port='.length);
const frontendPort = process.env.FRONTEND_PORT || cliPort;
const defaultServerUrl =
  String(frontendPort) === '3002'
    ? 'http://localhost:3001'
    : 'http://localhost:3000';
const serverUrl =
  process.env.DEV_PROXY_TARGET ||
  process.env.VITE_REACT_APP_SERVER_URL ||
  defaultServerUrl;
const isQuietBuild = process.env.VITE_QUIET_BUILD === '1';
const shouldReportCompressedSize = process.env.VITE_BUILD_STATS === '1';

// https://vitejs.dev/config/
export default defineConfig({
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  plugins: [
    codeInspectorPlugin({
      bundler: 'vite',
    }),
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
    force: true,
    esbuildOptions: {
      loader: {
        '.js': 'jsx',
        '.json': 'json',
      },
    },
  },
  build: {
    minify: 'esbuild',
    reportCompressedSize: shouldReportCompressedSize,
    chunkSizeWarningLimit: isQuietBuild ? 100000 : 500,
    rollupOptions: {
      onwarn(warning, warn) {
        if (
          warning.code === 'EVAL' &&
          warning.id?.includes('node_modules/lottie-web/')
        ) {
          return;
        }
        warn(warning);
      },
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
        },
      },
    },
  },
  server: {
    host: '0.0.0.0',
    proxy: {
      '/api': {
        target: serverUrl,
        changeOrigin: true,
        ws: true,
      },
      '/mj': {
        target: serverUrl,
        changeOrigin: true,
      },
      '/pg': {
        target: serverUrl,
        changeOrigin: true,
      },
    },
  },
});
