import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    include: ['src/**/*.component.test.tsx'],
    setupFiles: ['./src/test/setup.ts'],
    clearMocks: true,
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json-summary'],
      include: ['src/**/*.{ts,tsx}'],
      exclude: ['src/**/*.test.ts', 'src/**/*.test.tsx', 'src/test/**'],
      thresholds: {
        branches: 1,
        functions: 1,
        lines: 1,
        statements: 1,
      },
    },
  },
});
