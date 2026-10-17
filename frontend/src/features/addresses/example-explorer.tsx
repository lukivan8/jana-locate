import { useState } from 'react'
import { normalize, type InputRecord, type Result } from '../../api/client'
import { AnomalyExamples } from './anomaly-examples'

export function ExampleExplorer() {
  const [busy, setBusy] = useState(false)
  const [results, setResults] = useState<Result[]>([])
  const [error, setError] = useState('')
  async function run(records: InputRecord[]) {
    setBusy(true); setResults([]); setError('')
    try { setResults(await normalize(records)) }
    catch (e) { setError(e instanceof Error ? e.message : String(e)) }
    finally { setBusy(false) }
  }
  return <main id="main">
    <div className="intro"><div><h1>Сложные адреса для проверки разбора</h1><p>Выберите сценарий, посмотрите искусственные изменения и отправьте исходную строку в API. Эталонных ответов и оценки точности в этом наборе нет.</p></div></div>
    {error && <div role="alert" className="message error">{error} Повторите запрос.</div>}
    <AnomalyExamples busy={busy} results={results} onNormalize={run} />
  </main>
}
