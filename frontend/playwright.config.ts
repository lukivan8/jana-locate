import { defineConfig } from '@playwright/test'
const testURL = process.env.JANA_TEST_WEB_URL ?? 'http://localhost:3000'
export default defineConfig({
  testDir: './tests', fullyParallel: false, workers: 1,
  use: { baseURL: testURL, headless: true, screenshot: 'only-on-failure' },
  webServer: { command: `npm run dev -- --port ${new URL(testURL).port || '3000'}`, url: testURL, reuseExistingServer: !process.env.CI },
})
