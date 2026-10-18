import { test, expect } from '@playwright/test'

for (const width of [1440, 390]) {
  test(`contextual result preserves evidence and shows uncalibrated confidence at ${width}px`, async ({ page, context }) => {
    test.skip(process.env.JANA_TEST_CONTEXTUAL !== '1', 'Requires the sealed contextual service and a disposable job store')
    test.setTimeout(90_000)
    await page.setViewportSize({ width, height: width === 390 ? 844 : 1000 })
    const api = process.env.JANA_TEST_API_URL!
    await context.route('**/config.js', route => route.fulfill({ contentType: 'text/javascript', body: `window.JANA_CONFIG = ${JSON.stringify({ apiBaseUrl: api })}` }))
    await page.goto('/')
    await page.getByLabel('Один адрес на строку').fill('г. Астана, ул. Пробная, д. 19, за рынком Весна, кв 34')
    await page.getByRole('button', { name: 'Нормализовать пакет', exact: true }).click()
    await expect(page.locator('tbody > tr').first()).toContainText('Не оценена', { timeout: 30_000 })
    await expect(page.getByLabel('Ориентир', { exact: true })).toHaveValue('за рынком Весна', { timeout: 20_000 })
    await expect(page.getByLabel('Дом', { exact: true })).toHaveValue('19')
    await expect(page.getByLabel('Квартира', { exact: true })).toHaveValue('34')
    await expect(page.getByText('0%', { exact: true })).toHaveCount(0)
    const downloadPromise = page.waitForEvent('download')
    await page.getByRole('button', { name: 'Экспорт JSON', exact: true }).click()
    const stream = await (await downloadPromise).createReadStream()
    const chunks: Buffer[] = []; for await (const chunk of stream!) chunks.push(chunk)
    const result = JSON.parse(Buffer.concat(chunks).toString()).results[0]
    expect(result.normalizer_version).toBe('contextual-v7-grounded-microalpha-lean-v1')
    expect(result.confidence_available).toBe(false)
    expect(result.normalization_details.spans.length).toBeGreaterThan(0)
    expect(result.raw_address).toBe('г. Астана, ул. Пробная, д. 19, за рынком Весна, кв 34')
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })
}
