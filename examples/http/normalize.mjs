// Node.js 22.12+. Запуск: node examples/http/normalize.mjs
import { readFile } from 'node:fs/promises'
const base = (process.env.JANA_API_URL ?? 'http://localhost:8080').replace(/\/$/, '')
try {
  const input = JSON.parse(await readFile(new URL('./request.json', import.meta.url), 'utf8'))
  const response = await fetch(`${base}/v1/normalize`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input), signal: AbortSignal.timeout(300_000),
  })
  const body = await response.text()
  if (!response.ok) throw new Error(`HTTP ${response.status}: ${body}`)
  const output = JSON.parse(body)
  if (!Array.isArray(output.results) || output.results.length !== input.records.length)
    throw new Error('API вернул неожиданное количество результатов')
  output.results.forEach((row, i) => {
    if (row.id !== input.records[i].id || row.raw_address !== input.records[i].raw_address)
      throw new Error('API не сохранил исходный ID или адрес')
  })
  console.log(JSON.stringify(output, null, 2))
} catch (error) {
  console.error(`Ошибка нормализации: ${error.message}`)
  process.exitCode = 1
}
