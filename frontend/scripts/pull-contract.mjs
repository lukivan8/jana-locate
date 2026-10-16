import { writeFile } from 'node:fs/promises'
const base = process.env.JANA_API_URL ?? 'http://localhost:8080'
const response = await fetch(`${base.replace(/\/$/, '')}/openapi.json`)
if (!response.ok) throw new Error(`Contract download failed: ${response.status}`)
const contract = await response.json()
if (contract.openapi !== '3.0.3' || contract.info?.title !== 'Jana Address API') throw new Error('Unexpected contract')
await writeFile(new URL('../src/api/openapi.json', import.meta.url), `${JSON.stringify(contract, null, 2)}\n`)
console.log('Contract saved. Run npm run contract:generate and review the diff.')
