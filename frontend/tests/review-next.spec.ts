import { test, expect } from '@playwright/test'
import type { components } from '../src/api/local-schema.gen'

for (const width of [1440, 390]) {
  for (const filtered of [false, true]) {
    test(`confirmation advances, wraps and finishes the review queue at ${width}px filtered=${filtered}`, async ({ page }) => {
      await page.setViewportSize({ width, height: width === 390 ? 844 : 1000 })
      const api = process.env.JANA_TEST_API_URL ?? 'http://localhost:8080'
      await page.route('**/config.js', route => route.fulfill({ contentType: 'text/javascript', body: `window.JANA_CONFIG = ${JSON.stringify({ apiBaseUrl: api })}` }))
      await page.goto('/')
      await page.getByLabel('Один адрес на строку').fill('Неизвестный первый\nКөкшетау, ул.Әуезов 15 кв6\nНеизвестный средний\nНеизвестный последний')
      await page.getByRole('button', { name: 'Нормализовать пакет', exact: true }).click()
      await expect(page.getByRole('button', { name: 'Проверить', exact: true })).toHaveCount(3)
      if (filtered) await page.getByLabel('Только требующие проверки').check()
      await page.getByRole('button', { name: 'Проверить', exact: true }).last().click()
      await page.getByLabel('Ориентир', { exact: true }).fill('Проверенный последний')
      await page.getByRole('button', { name: 'Подтвердить адрес', exact: true }).click()
      await expect(page.locator('#correction-heading')).toHaveText('Неизвестный первый')
      await expect(page.locator('#correction-heading')).toBeFocused()
      await expect(page.getByLabel('Ориентир', { exact: true })).toHaveValue('')
      await page.getByLabel('Квартира', { exact: true }).fill('77')
      await page.getByRole('button', { name: 'Подтвердить адрес', exact: true }).click()
      await expect(page.locator('#correction-heading')).toHaveText('Неизвестный средний')
      await expect(page.getByLabel('Квартира', { exact: true })).toHaveValue('')
      await page.getByRole('button', { name: 'Подтвердить адрес', exact: true }).click()
      await expect(page.locator('#editor')).toHaveCount(0)
      await expect(page.getByRole('button', { name: 'Проверить', exact: true })).toHaveCount(0)
      if (filtered) await expect(page.getByText('Все адреса обработаны. Очередь проверки пуста.')).toBeVisible()
      await page.reload()
      await expect(page.locator('#rows > tr')).toHaveCount(4)
      await expect(page.getByRole('img', { name: 'Проверено оператором', exact: true })).toHaveCount(3)
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    })
  }
}

for (const filtered of [false, true]) {
  test(`CLI confirmation keeps stable positions across pages filtered=${filtered}`, async ({ page }) => {
    const empty = { province: null, city: null, district: null, living_area: null, street_type: null, street_name: null, house: null, apartment: null, entrance: null, floor: null, landmark: null }
    const rows: components['schemas']['Row'][] = Array.from({ length: 152 }, (_, position) => ({
      position, source_id: String(position), raw_address: `Адрес ${position}`, reviewed: false, revision: 1,
      result: { ...empty, id: position, raw_address: `Адрес ${position}`, status: position <= 100 || position === 151 ? 'needs_review' : 'matched', confidence: .5, warnings: [], coordinates: null, catalog_id: null, catalog_revision: null },
    }))
    const run: components['schemas']['Run'] = { id: 'next-review', source: { kind: 'file', schema: '', table: '', key: '', column: '', target_schema: '', target_table: '' }, status: 'review', total: rows.length, processed: rows.length, reviewed: 0, revision: 1, created_at: '2026-10-17T00:00:00Z', updated_at: '2026-10-17T00:00:00Z', error: '', api_url: 'http://localhost:18088' }
    const pending = () => rows.filter(row => !row.reviewed && row.result.status !== 'matched')
    await page.route('**/config.js', route => route.fulfill({ contentType: 'text/javascript', body: 'window.JANA_CONFIG = {apiBaseUrl:"http://local-review.test", workspace:"local"}' }))
    await page.route('http://local-review.test/**', async route => {
      const request = route.request(), url = new URL(request.url())
      if (url.pathname === '/v1/events') return route.fulfill({ contentType: 'text/event-stream', body: `data: ${JSON.stringify({ runs: [run] })}\n\n` })
      if (url.pathname.endsWith('/summary')) return route.fulfill({ json: { total: rows.length, needs_review: pending().length, reviewed: run.reviewed, automatic: 50 } })
      if (url.pathname.endsWith('/candidates')) return route.fulfill({ json: { candidates: [], revision: 1, reference_version: 'test' } })
      const review = url.pathname.match(/\/rows\/(\d+)\/review$/)
      if (review) {
        const row = rows[Number(review[1])]
        expect(request.postDataJSON().expected_revision).toBe(row.revision)
        row.result = { ...row.result, ...request.postDataJSON().fields }
        row.reviewed = true; row.revision++; run.reviewed++; run.revision++
        return route.fulfill({ json: row })
      }
      if (url.pathname.endsWith('/rows')) {
        const offset = Number(url.searchParams.get('offset') ?? 0)
        const list = url.searchParams.get('review_only') === 'true' ? pending() : rows
        return route.fulfill({ json: { rows: list.slice(offset, offset + 50), total: rows.length } })
      }
      return route.fulfill({ json: run })
    })
    await page.goto('/?job=next-review')
    await expect(page.locator('#rows > tr')).toHaveCount(50)
    if (filtered) await page.getByLabel('Только требующие проверки').check()
    await page.getByRole('button', { name: 'Далее', exact: true }).click()
    await expect(page.locator('#rows > tr').last()).toContainText('Адрес 99')
    await page.locator('#rows > tr').last().getByRole('button').click()
    for (const next of [100, 151, 0]) {
      await page.getByRole('button', { name: 'Подтвердить адрес', exact: true }).click()
      await expect(page.locator('#correction-heading')).toHaveText(`Адрес ${next}`)
      await expect(page.locator('#correction-heading')).toBeFocused()
    }
    expect(rows[99].reviewed && rows[100].reviewed && rows[151].reviewed).toBe(true)
    expect(rows[0].reviewed).toBe(false)
  })
}
