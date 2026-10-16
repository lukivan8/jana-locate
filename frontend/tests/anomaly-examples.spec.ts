import { readFileSync } from 'node:fs'
import { test, expect } from '@playwright/test'

const dataset = JSON.parse(readFileSync(new URL('../src/data/address-anomalies.json', import.meta.url), 'utf8')) as {
  synthetic: boolean
  categories: { id: string; source: string }[]
  records: { id: string; raw_address: string }[]
  scenarios: { id: string; anomalies: string[] }[]
}

test('synthetic fixture covers every category and every pair of named anomalies', () => {
  expect(dataset.synthetic).toBe(true)
  expect(new Set(dataset.records.map(record => record.id)).size).toBe(dataset.records.length)
  expect(dataset.records.map(record => record.id)).toEqual(dataset.scenarios.map(scenario => scenario.id))
  for (const category of dataset.categories) {
    const isolated = dataset.scenarios.filter(scenario => scenario.anomalies.length === 1 && scenario.anomalies[0] === category.id)
    expect(isolated.length, category.id).toBeGreaterThanOrEqual(5)
  }
  const primary = dataset.categories.filter(category => category.source === 'presentation')
  expect(primary).toHaveLength(9)
  for (let i = 0; i < primary.length; i++) {
    for (let j = i + 1; j < primary.length; j++) {
      expect(dataset.scenarios.some(scenario => scenario.anomalies.length === 2 && scenario.anomalies.includes(primary[i].id) && scenario.anomalies.includes(primary[j].id))).toBe(true)
    }
  }
  for (const record of dataset.records) expect(Buffer.byteLength(record.raw_address)).toBeLessThanOrEqual(4096)
  expect(dataset.records.some(record => record.raw_address === '')).toBe(true)
  expect(dataset.records.some(record => record.raw_address.includes('\n'))).toBe(true)
})

for (const viewport of [{ width: 1440, height: 1000 }, { width: 390, height: 844 }]) {
  test(`select, normalize and export an individual example at ${viewport.width}px`, async ({ page }) => {
    await page.setViewportSize(viewport)
    const api = process.env.JANA_TEST_API_URL ?? 'http://localhost:8080'
    await page.route('**/config.js', route => route.fulfill({ contentType: 'text/javascript', body: `window.JANA_CONFIG = ${JSON.stringify({ apiBaseUrl: api })}` }))
    await page.goto('/')
    await page.getByText('Исследовать тестовые примеры', { exact: true }).click()
    const examples = page.getByRole('region', { name: 'Проверить на примерах' })
    await expect(examples.getByRole('button', { name: 'Нормализовать адрес', exact: true })).toBeVisible()
    await expect(examples.getByRole('list', { name: 'Тестовые адреса' }).getByRole('button')).toHaveCount(8)
    await examples.getByRole('button', { name: 'Следующий адрес', exact: true }).click()
    await expect(examples.getByRole('button', { pressed: true })).toContainText('synthetic_002')
    await examples.getByRole('button', { name: 'Предыдущий адрес', exact: true }).click()
    await expect(examples.getByRole('button', { pressed: true })).toContainText('synthetic_001')
    await examples.getByRole('button', { name: 'Далее', exact: true }).click()
    await expect(examples.getByRole('button', { pressed: true })).toContainText('synthetic_009')
    await examples.getByRole('button', { name: 'Назад', exact: true }).click()
    await examples.getByLabel('Тип аномалии').selectOption('invisible')
    await examples.getByLabel('Найти пример').fill('synthetic_069')
    const input = dataset.records.find(record => record.id === 'synthetic_069')!
    await examples.getByRole('list', { name: 'Тестовые адреса' }).getByRole('button').click()
    await expect(examples.getByLabel('Исходный тестовый адрес')).toContainText('[LF]')
    const request = page.waitForRequest(request => request.url().endsWith('/v1/normalize') && request.method() === 'POST')
    const response = page.waitForResponse(response => response.url().endsWith('/v1/normalize'))
    await examples.getByRole('button', { name: 'Нормализовать адрес', exact: true }).click()
    expect((await request).postDataJSON()).toEqual({ records: [input] })
    const output = await (await response).json()
    expect(output.results).toHaveLength(1)
    expect(output.results[0].raw_address).toBe(input.raw_address)
    await expect(examples.getByLabel('Результат тестового адреса')).toBeVisible()
    await expect(page.locator('tbody tr')).toHaveCount(1)
    const downloadPromise = page.waitForEvent('download')
    await page.getByRole('button', { name: 'Экспорт JSON', exact: true }).click()
    const download = await downloadPromise
    const stream = await download.createReadStream()
    const chunks: Buffer[] = []
    for await (const chunk of stream!) chunks.push(chunk)
    const exported = JSON.parse(Buffer.concat(chunks).toString())
    expect(exported.results).toHaveLength(1)
    expect(exported.results[0].id).toBe(input.id)
    expect(exported.results[0].raw_address).toBe(input.raw_address)

    await examples.getByLabel('Найти пример').fill('')
    await examples.getByLabel('Тип аномалии').selectOption('unparseable')
    await expect(examples.getByLabel('Результат тестового адреса')).toHaveCount(0)
    await expect(examples.getByLabel('Исходный тестовый адрес')).toContainText('[пустая строка]')
    await page.route('**/v1/normalize', route => route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ code: 'test_failure', message: 'Test failure: retry the request.' }) }), { times: 1 })
    await examples.getByRole('button', { name: 'Нормализовать адрес', exact: true }).click()
    await expect(page.getByRole('alert')).toContainText('Test failure')
    await expect(examples.getByRole('button', { name: 'Нормализовать адрес', exact: true })).toBeEnabled()
    await examples.getByRole('button', { name: 'Нормализовать адрес', exact: true }).click()
    await expect(examples.getByLabel('Результат тестового адреса')).toBeVisible()
    await expect(examples.getByLabel('Результат тестового адреса')).toContainText('null')
    await expect(page.getByRole('alert')).toHaveCount(0)

    await examples.getByLabel('Тип аномалии').selectOption('all')
    await examples.getByLabel('Только комбинации').check()
    await expect(examples.getByText('Найдено: 59', { exact: false })).toBeVisible()
    await examples.getByLabel('Найти пример').fill('несуществующий-адрес')
    await expect(examples.getByText('Примеров не найдено. Измените поиск или фильтр.')).toBeVisible()
    await expect(examples.getByRole('button', { name: 'Нормализовать адрес', exact: true })).toHaveCount(0)
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })
}
