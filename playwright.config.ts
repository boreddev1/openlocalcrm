import { defineConfig, devices } from '@playwright/test';
import path from 'path';

const authFile = path.resolve(__dirname, 'playwright/.auth/user.json');

// Port of the demo backend started below. 8080 is the documented default and stays
// authoritative in CI, but it is not universally free on a developer machine (an
// IDE server or another service may already own it), so it can be moved:
//
//   E2E_BACKEND_PORT=8090 make test-e2e
//
// This must stay in sync with the proxy target in web/vite.config.ts, which reads
// the same variable: the SPA only reaches the API through that proxy, so moving
// one port without the other makes every API spec fail against a dead listener.
const backendPort = process.env.E2E_BACKEND_PORT ?? '8080';
const backendBaseUrl = `http://localhost:${backendPort}`;

export default defineConfig({
  testDir: './e2e',
  timeout: 30 * 1000,
  expect: {
    timeout: 5000,
  },
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: 'list',
  use: {
    baseURL: 'http://localhost:5173',
    trace: 'on-first-retry',
  },
  webServer: [
    {
      command: `DEMO_MODE=true PORT=${backendPort} ./bin/crm-server`,
      url: `${backendBaseUrl}/api/v1/health`,
      reuseExistingServer: !process.env.CI,
      timeout: 120 * 1000,
    },
    {
      command: 'cd web && pnpm dev',
      url: 'http://localhost:5173',
      reuseExistingServer: !process.env.CI,
      timeout: 120 * 1000,
    },
  ],
  projects: [
    {
      name: 'setup',
      testMatch: /.*\.setup\.ts/,
    },
    {
      name: 'chromium',
      testMatch: /.*\.spec\.ts/,
      use: {
        ...devices['Desktop Chrome'],
        storageState: authFile,
      },
      dependencies: ['setup'],
    },
  ],
});
