import type { InputRecord } from '../../api/client'

export function addressLines(records: InputRecord[]): string {
  return records.map(record => record.raw_address.replace(/[\r\n]+/g, ' ')).join('\n')
}

function validate(records: InputRecord[]): InputRecord[] {
  if (!records.length || records.length > 1000) throw new Error('В файле должно быть от 1 до 1 000 адресов.')
  return records
}

// CSV quoting is structural; punctuation inside an address is preserved.
export function csvRows(text: string): string[][] {
  // Count separators outside quotes in the first logical row.
  const counts = new Map([[',', 0], [';', 0], ['\t', 0]])
  let inQuotes = false
  for (let i = 0; i < text.length; i++) {
    const char = text[i]
    if (char === '"') {
      if (inQuotes && text[i + 1] === '"') i++
      else inQuotes = !inQuotes
    } else if (!inQuotes) {
      if (char === '\r' || char === '\n') break
      if (counts.has(char)) counts.set(char, counts.get(char)! + 1)
    }
  }
  const delimiter = [...counts].sort((a, b) => b[1] - a[1])[0][0]
  const rows: string[][] = []
  let row: string[] = [], cell = '', quoted = false
  for (let i = 0; i < text.length; i++) {
    const char = text[i]
    if (char === '"') {
      if (quoted && text[i + 1] === '"') { cell += '"'; i++ }
      else if (quoted || cell === '') quoted = !quoted
      else throw new Error('Некорректные кавычки CSV. Заключите адреса с разделителями в двойные кавычки.')
    } else if (char === delimiter && !quoted) { row.push(cell); cell = '' }
    else if ((char === '\n' || char === '\r') && !quoted) {
      if (char === '\r' && text[i + 1] === '\n') i++
      row.push(cell); if (row.some(value => value.trim())) rows.push(row)
      row = []; cell = ''
    } else cell += char
  }
  if (quoted) throw new Error('В CSV не закрыты двойные кавычки.')
  row.push(cell); if (row.some(value => value.trim())) rows.push(row)
  return rows
}

export function importAddresses(name: string, content: string): InputRecord[] {
  const text = content.replace(/^\uFEFF/, '')
  const extension = name.split('.').pop()?.toLowerCase()
  if (extension === 'txt') return validate(text.split(/\r?\n/).filter(line => line.trim()).map((raw_address, i) => ({ id: String(i + 1), raw_address })))
  if (extension === 'csv') {
    const rows = csvRows(text)
    return addressesFromColumn(rows, Math.max(0, rows[0]?.findIndex(cell => cell.trim().toLowerCase() === 'raw_address') ?? 0), true)
  }
  if (extension !== 'json') throw new Error('Выберите файл TXT, JSON, CSV или XLSX.')
  let value: unknown
  try { value = JSON.parse(text) } catch { throw new Error('Некорректный JSON. Проверьте формат файла.') }
  const records = Array.isArray(value) ? value : (value as { records?: unknown })?.records
  if (!Array.isArray(records)) throw new Error('JSON: массив адресов или { records: [...] } с полем raw_address.')
  return validate(records.map((record, i) => {
    if (typeof record === 'string') return { id: String(i + 1), raw_address: record }
    if (!record || typeof record !== 'object' || typeof record.raw_address !== 'string') throw new Error('Каждой записи JSON нужно строковое поле raw_address.')
    const id: unknown = record.id ?? String(i + 1)
    if (!(typeof id === 'string' || (typeof id === 'number' && Number.isSafeInteger(id)))) throw new Error('ID должен быть строкой или безопасным целым числом. Большие ID передавайте строками.')
    return { id, raw_address: record.raw_address }
  }))
}

export function addressesFromColumn(rows: string[][], column: number, hasHeader: boolean): InputRecord[] {
  const width = rows[0]?.length ?? 0
  if (column < 0 || column >= width) throw new Error('Выберите колонку с адресами.')
  const header = hasHeader ? rows[0].map(cell => cell.trim().toLowerCase()) : []
  const idIndex = header.indexOf('id')
  return validate(rows.slice(hasHeader ? 1 : 0).map((row, i) => {
    if (row.length !== width) throw new Error(`Строка CSV ${i + (hasHeader ? 2 : 1)}: число колонок не совпадает с первой строкой.`)
    return { id: idIndex < 0 || idIndex === column ? String(i + 1) : row[idIndex], raw_address: row[column] }
  }))
}

export type AddressImport = { records: InputRecord[] } | { rows: string[][]; sheetName?: string }

export async function readAddressFile(file: File): Promise<AddressImport> {
  if (file.size > 2 * 1024 * 1024) throw new Error('Файл превышает 2 MiB.')
  const extension = file.name.split('.').pop()?.toLowerCase()
  let text: string
  let sheetName: string | undefined
  if (extension === 'xlsx') {
    const { read, utils } = await import('xlsx')
    try {
      const workbook = read(await file.arrayBuffer(), { type: 'array' })
      sheetName = workbook.SheetNames[0]
      if (!sheetName) throw new Error('empty workbook')
      text = utils.sheet_to_csv(workbook.Sheets[sheetName], { blankrows: false })
    } catch { throw new Error('Не удалось прочитать XLSX. Проверьте файл и сохраните его повторно.') }
  } else {
    text = await file.text()
    if (extension !== 'csv') return { records: importAddresses(file.name, text) }
  }
  const rows = csvRows(text.replace(/^\uFEFF/, ''))
  if (!rows.length) throw new Error('В файле нет адресов.')
  if (rows.length > 1001) throw new Error('В файле должно быть от 1 до 1 000 адресов.')
  if (rows[0].length > 1) return { rows, sheetName }
  return { records: addressesFromColumn(rows, 0, /^(raw_address|address|адрес)$/i.test(rows[0][0].trim())) }
}
