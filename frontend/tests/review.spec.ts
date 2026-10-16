import { test, expect } from '@playwright/test'

test('browser normalizes through HTTP, confirms, uploads and exports', async ({ page }) => {
  const api = process.env.JANA_TEST_API_URL ?? 'http://localhost:8080'
  await page.route('**/config.js', route => route.fulfill({ contentType: 'text/javascript', body: `window.JANA_CONFIG = ${JSON.stringify({ apiBaseUrl: api })}` }))
  await page.goto('/')
  await expect(page.getByText('3', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Нормализовать пакет' }).click()
  await expect(page.locator('tbody tr')).toHaveCount(4)
  await expect(page.locator('tbody tr').first()).toContainText('Курильская, 9')
  await expect(page.locator('tbody tr').last()).toContainText(/Не распознан|Нужна проверка/)
  await page.getByRole('button', { name: /Проверить|Просмотреть/, exact: true }).first().click()
  // Use a synthetic identity absent from the demo seed so this exercises creation.
  await page.getByLabel('Город', { exact: true }).fill('Тестовый город')
  await page.getByLabel('Область', { exact: true }).fill('Тестовая область')
  await page.getByLabel('Широта', { exact: true }).fill('43.24')
  await page.getByLabel('Долгота', { exact: true }).fill('76.89')
  await page.getByRole('button', { name: 'Подтвердить здание и сохранить' }).click()
  await expect(page.getByRole('status').filter({ hasText: 'Здание сохранено' })).toContainText('Здание сохранено')
  await expect(page.locator('tbody tr').first()).toContainText('Найден в каталоге')
  // Reload discards session state but must preserve the building confirmation.
  await page.reload()
  await page.getByLabel('Один адрес на строку').fill('г. Тестовый город, ул. Курильская 9 кв 1\nТуран 55/6 магазин Береке 4подьезд 9этаж\n66квартал 12дом 69квартира\nНеизвестный адрес')
  await page.getByRole('button', { name: 'Нормализовать пакет' }).click()
  await expect(page.locator('tbody tr').first()).toContainText('Найден в каталоге')
  await page.getByLabel('Только требующие проверки').check()
  await expect(page.locator('tbody tr')).toHaveCount(3)
  await page.getByLabel('Только требующие проверки').uncheck()
  await page.getByLabel('Загрузить JSON').setInputFiles({ name: 'batch.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify([{ id: 'external', raw_address: 'г. Тестовый город, ул. Курильская 9 кв 99' }])) })
  await page.getByRole('button', { name: 'Нормализовать пакет' }).click()
  await expect(page.locator('tbody tr')).toHaveCount(1)
  await expect(page.locator('tbody tr')).toContainText('Найден в каталоге')
  const downloading = page.waitForEvent('download')
  await page.getByRole('button', { name: 'Экспорт JSON' }).click()
  const download = await downloading
  expect(download.suggestedFilename()).toBe('jana-address-results.json')
  const stream = await download.createReadStream()
  const chunks = []; for await (const chunk of stream!) chunks.push(chunk)
  const result = JSON.parse(Buffer.concat(chunks).toString()).results[0]
  expect(result.apartment).toBe('99')
  expect(result.id).toBe('external')
  expect(result.coordinates).toEqual({ latitude: 43.24, longitude: 76.89 })
})
