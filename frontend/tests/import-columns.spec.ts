import { test, expect } from '@playwright/test'
import { utils, write } from 'xlsx'

for (const width of [1440, 390]) {
  for (const format of ['csv', 'xlsx']) {
    test(`${format}: choose address column, cancel and reuse at ${width}px`, async ({ page }) => {
      await page.setViewportSize({ width, height: width === 390 ? 844 : 1000 })
      await page.goto('/')
      const input = page.getByLabel('Один адрес на строку')
      const file = page.getByLabel('Загрузить JSON, TXT, CSV или XLSX')
      await input.fill('Текущий адрес')
      const rows = [['id', 'Получатель', 'Адрес доставки'], ['001', 'Айжан', 'Караганда, Алалыкина 12; подъезд 2'], ['002', 'Олег', 'Сарань, Жамбыла 63']]
      const book = utils.book_new()
      utils.book_append_sheet(book, utils.aoa_to_sheet(rows), 'Доставка')
      const buffer = format === 'xlsx' ? write(book, { type: 'buffer', bookType: 'xlsx' }) : Buffer.from(utils.sheet_to_csv(book.Sheets['Доставка']))
      const upload = { name: `addresses.${format}`, mimeType: 'application/octet-stream', buffer }
      await file.setInputFiles(upload)
      const dialog = page.getByRole('dialog', { name: 'Выберите колонку с адресами' })
      await expect(dialog).toBeVisible()
      await expect(dialog.getByText('Адрес доставки', { exact: true })).toBeVisible()
      await page.getByRole('button', { name: 'Отмена', exact: true }).click()
      await expect(input).toHaveValue('Текущий адрес')
      // Nested above the expanded composer, then Escape restores that composer.
      await page.getByRole('button', { name: 'На весь экран' }).click()
      await file.setInputFiles(upload)
      await expect(dialog).toBeVisible()
      await page.keyboard.press('Escape')
      await expect(dialog).toHaveCount(0)
      await expect(input).toHaveValue('Текущий адрес')
      await file.setInputFiles(upload)
      await expect(dialog).toBeVisible()
      await dialog.getByRole('radio', { name: /^Адрес доставки/ }).check()
      expect(await dialog.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true)
      await page.getByRole('button', { name: 'Импортировать', exact: true }).click()
      await expect(input).toHaveValue(rows.slice(1).map(row => row[2]).join('\n'))
      // Check the actual submission, including original IDs, without mutating a production API.
      let submitted: unknown
      await page.route('**/v1/normalization-jobs', route => {
        submitted = route.request().postDataJSON()
        return route.fulfill({ status: 400, contentType: 'application/json', body: JSON.stringify({ message: 'Проверка отправки' }) })
      })
      await page.getByRole('button', { name: 'Нормализовать пакет' }).click()
      await expect.poll(() => submitted).toEqual({ records: rows.slice(1).map(row => ({ id: row[0], raw_address: row[2] })) })
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true)
    })
  }
  test(`headerless CSV and single column at ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 844 })
    await page.goto('/')
    const file = page.getByLabel('Загрузить JSON, TXT, CSV или XLSX')
    const input = page.getByLabel('Один адрес на строку')
    await file.setInputFiles({ name: 'no-header.csv', mimeType: 'text/csv', buffer: Buffer.from('1;"Караганда, дом 1"\n2;"Сарань, дом 2"') })
    const dialog = page.getByRole('dialog', { name: 'Выберите колонку с адресами' })
    await dialog.getByLabel('Первая строка — заголовки').uncheck()
    await dialog.getByRole('radio', { name: /^Колонка 2/ }).check()
    await page.getByRole('button', { name: 'Импортировать', exact: true }).click()
    await expect(input).toHaveValue('Караганда, дом 1\nСарань, дом 2')
    await file.setInputFiles({ name: 'single.csv', mimeType: 'text/csv', buffer: Buffer.from('raw_address\n"Караганда, дом 3"') })
    await expect(dialog).toHaveCount(0)
    await expect(input).toHaveValue('Караганда, дом 3')
    await file.setInputFiles({ name: 'broken.csv', mimeType: 'text/csv', buffer: Buffer.from('raw_address\n"не закрыто') })
    await expect(page.getByRole('alert')).toContainText('не закрыты')
    await expect(input).toHaveValue('Караганда, дом 3')
  })
}
