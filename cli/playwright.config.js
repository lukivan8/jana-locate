import { defineConfig } from '@playwright/test';
export default defineConfig({
  testDir: './tests/browser', workers: 1, fullyParallel: false,
  use: { baseURL: process.env.JANA_TEST_WEB_URL ?? 'http://localhost:8095', screenshot: 'only-on-failure',
    httpCredentials: process.env.JANA_WEB_TOKEN ? { username: 'jana', password: process.env.JANA_WEB_TOKEN, send: 'always' } : undefined },
});
