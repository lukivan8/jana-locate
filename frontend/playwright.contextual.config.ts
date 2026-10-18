import { defineConfig } from '@playwright/test'
import config from './playwright.config'
export default defineConfig({ ...config, expect: { timeout: 20_000 }, timeout: 90_000 })
