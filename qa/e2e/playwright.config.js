// NetBerth E2E — real-browser devil tests against an isolated QA instance.
// Run with NODE_PATH set to the global npm root (Playwright is installed
// globally in the dev environment), e.g.:
//   NODE_PATH="$(npm root -g)" playwright test --config qa/e2e/playwright.config.js
const { defineConfig } = require('playwright/test');

module.exports = defineConfig({
  testDir: './tests',
  timeout: 90_000,
  expect: { timeout: 15_000 },
  retries: 1,
  workers: 1,
  reporter: [['list']],
  use: {
    baseURL: process.env.NB_QA_BASE || 'http://127.0.0.1:18446',
    headless: true,
    viewport: { width: 1440, height: 900 },
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
    locale: 'en-US',
  },
  projects: [{ name: 'chromium', use: { browserName: 'chromium' } }],
});
