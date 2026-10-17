import { test, expect } from '@playwright/test'

for (const width of [1440, 390]) {
  test(`partial address coverage keeps settlement choice and recomputes district at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: width === 390 ? 844 : 1000 })
    const api = process.env.JANA_TEST_API_URL ?? 'http://localhost:8080'
    await page.route('**/config.js', route => route.fulfill({ contentType: 'text/javascript', body: `window.JANA_CONFIG = ${JSON.stringify({ apiBaseUrl: api })}` }))
    const raw = 'кв1, 3подъезд 2этаж, ул.Курильская 9, Алматы, магазин Береке'
    await page.goto('/')
    await page.getByLabel('Один адрес на строку').fill(raw)
    await page.getByRole('button', { name: 'Нормализовать пакет', exact: true }).click()
    const row = page.locator('tbody > tr').first()
    await expect(row.getByRole('img', { name: 'Требует проверки', exact: true })).toBeVisible()
    await expect(page.getByText('Город неоднозначен в справочнике', { exact: true })).toBeVisible()
    await expect(page.getByLabel('Район', { exact: true })).toHaveValue('')
    await expect(page.getByLabel('Область', { exact: true })).toHaveValue('')
    await expect(page.getByRole('radio')).toHaveCount(2)
    await expect(page.getByRole('radio', { checked: true })).toHaveCount(0)
    await page.getByRole('radio', { name: /Алматы · город/ }).check()
    await page.getByRole('button', { name: 'Выбрать населённый пункт', exact: true }).click()
    await expect(page.getByLabel('Район', { exact: true })).toHaveValue('Алмалинский')
    await expect(page.getByLabel('Область', { exact: true })).toHaveValue('Алматы')
    await expect(row.getByRole('img', { name: 'Нормализован', exact: true })).toBeVisible()
    await page.getByRole('button', { name: 'Исправить', exact: true }).click()
    await page.getByRole('radio', { name: /Алматинское · село/ }).check()
    await page.getByRole('button', { name: 'Выбрать населённый пункт', exact: true }).click()
    await expect(page.getByLabel('Район', { exact: true })).toHaveValue('Есильский')
    await expect(page.getByLabel('Область', { exact: true })).toHaveValue('Акмолинская область')
    await expect(page.getByLabel('Квартира', { exact: true })).toHaveValue('1')
    await expect(page.getByLabel('Подъезд', { exact: true })).toHaveValue('3')
    await expect(page.getByLabel('Этаж', { exact: true })).toHaveValue('2')
    await expect(page.getByLabel('Один адрес на строку')).toHaveValue(raw)
    await page.reload()
    await expect(page.getByLabel('Район', { exact: true })).toHaveValue('Есильский')
    await expect(page.getByLabel('Область', { exact: true })).toHaveValue('Акмолинская область')
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
  })
}
