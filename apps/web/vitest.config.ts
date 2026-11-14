import { fileURLToPath, URL } from 'node:url';

import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./Src', import.meta.url)) },
  },
  test: {
    environment: 'jsdom',
    globals: false,
    setupFiles: ['./Src/Core/Testing/Setup.ts'],
    include: ['Src/**/*.test.{ts,tsx}', 'E2E/**/*.test.ts', 'Scripts/**/*.test.mjs'],
    css: false,
    restoreMocks: true,
  },
});
