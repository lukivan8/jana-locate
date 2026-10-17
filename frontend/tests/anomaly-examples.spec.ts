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
  expect(dataset.records).toHaveLength(2000)
  expect(new Set(dataset.records.map(record => record.raw_address)).size).toBe(2000)
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


test('downloadable input packets retain every raw string and respect API limits', () => {
  const first = JSON.parse(readFileSync(new URL('../src/data/address-anomalies-part-1.json', import.meta.url), 'utf8'))
  const second = JSON.parse(readFileSync(new URL('../src/data/address-anomalies-part-2.json', import.meta.url), 'utf8'))
  expect(first).toHaveLength(1000)
  expect(second).toHaveLength(1000)
  expect([...first, ...second]).toEqual(dataset.records)
  for (const packet of [first, second]) {
    expect(Buffer.byteLength(JSON.stringify({ records: packet }))).toBeLessThan(2 * 1024 * 1024)
    for (const record of packet) expect(Object.keys(record).sort()).toEqual(['id', 'raw_address'])
  }
  const source = JSON.parse(readFileSync(new URL('../src/data/address-bases.json', import.meta.url), 'utf8'))
  expect(source.bases).toHaveLength(920)
  expect(source.settlement_count).toBeGreaterThan(500)
  expect(source.license).toBe('ODbL-1.0')
  const scenarios = JSON.parse(readFileSync(new URL('../src/data/address-anomalies.json', import.meta.url), 'utf8')).scenarios
  const ids = new Set(source.bases.map((base: {id: string}) => base.id))
  const derived = scenarios.filter((scenario: {source_base_id?: string}) => scenario.source_base_id)
  expect(derived).toHaveLength(1839)
  for (const scenario of derived) expect(ids.has(scenario.source_base_id)).toBe(true)
  for (const tag of ['transliteration', 'mixed_script', 'keyboard']) {
    expect(scenarios.filter((scenario: {anomalies: string[]}) => scenario.anomalies.includes(tag)).length).toBeGreaterThan(50)
  }
})

for (const width of [1440, 390]) {
  test(`source-based transliteration examples, API and downloads at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 1000 })
    const api = process.env.JANA_TEST_API_URL ?? 'http://localhost:8080'
    await page.route('**/config.js', route => route.fulfill({ contentType: 'text/javascript', body: `window.JANA_CONFIG = ${JSON.stringify({ apiBaseUrl: api })}` }))
    await page.goto('/geocoder')
    await page.getByRole('link', { name: 'Исследовать 2000 синтетических примеров' }).click()
    const examples = page.getByRole('region', { name: 'Проверить на примерах' })
    await examples.getByLabel('Тип аномалии').selectOption('transliteration')
    const records = examples.getByRole('list', { name: 'Тестовые адреса' }).getByRole('button')
    await expect(records).toHaveCount(8)
    await records.first().click()
    await expect(examples.getByRole('link', { name: 'Объект OSM, использованный как основа' })).toBeVisible()
    const raw = examples.getByLabel('Исходный тестовый адрес')
    await expect(raw).toContainText(/[A-Za-z]/)
    const request = page.waitForRequest(r => r.url().endsWith('/v1/normalize') && r.method() === 'POST')
    await examples.getByRole('button', { name: 'Нормализовать адрес', exact: true }).click()
    const body = (await request).postDataJSON()
    expect(body.records).toHaveLength(1)
    const fixture = dataset.records.find(record => record.id === body.records[0].id)
    expect(body.records[0]).toEqual(fixture)
    await expect(examples.getByRole('region', { name: 'Результат тестового адреса' })).toBeVisible()
    const download = page.waitForEvent('download')
    await examples.getByRole('link', { name: 'Скачать пакет 2 · 1000 адресов', exact: true }).click()
    const stream = await (await download).createReadStream()
    const chunks: Buffer[] = []; for await (const chunk of stream!) chunks.push(chunk)
    const packet = JSON.parse(Buffer.concat(chunks).toString())
    expect(packet).toHaveLength(1000)
    expect(packet[0].id).toBe('synthetic_1001')
    await examples.getByLabel('Найти пример').fill('несуществующий пример')
    await expect(examples).toContainText('Примеров не найдено')
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })
}
