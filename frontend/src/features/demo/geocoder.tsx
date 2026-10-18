import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import { normalize, type Result } from '../../api/client'
import { Button, TextArea } from '../../ui/material'
import { ResultDetails } from '../addresses/result-details'

export function Geocoder() {
  const [text, setText] = useState('')
  const [result, setResult] = useState<Result | null>(null)
  const [busy, setBusy] = useState(false)
  const [elapsed, setElapsed] = useState<number | null>(null)
  const [error, setError] = useState('')
  async function run() {
    setBusy(true); setError(''); setResult(null); setElapsed(null)
    const started = performance.now()
    try { const parsed = await normalize([{ id: 'live-input', raw_address: text }]); setResult(parsed[0]); setElapsed(Math.round(performance.now() - started)) }
    catch (e) { setError(e instanceof Error ? e.message : String(e)) }
    finally { setBusy(false) }
  }
  return <main id="main"><div className="intro"><div><h1>Любой адрес. Разбор на ваших глазах.</h1><p>Введите свой адрес с сокращениями, опечатками или деталями доставки. Сервис выделит адресные поля, уточнит их по справочникам и покажет неоднозначности.</p></div></div>
    <p><Link to="/examples">Исследовать 2000 синтетических примеров</Link></p>
    <div className="normalization-input"><div><section className="panel"><form onSubmit={e => { e.preventDefault(); void run() }} aria-busy={busy}>
      <TextArea label="Адрес в свободной форме" value={text} onChange={e => { setText(e.target.value); setResult(null); setElapsed(null); setError('') }} disabled={busy} rows={4} placeholder="Например: Астана, Туран 55/6, кв 12" supportingText="Один адрес. В обработку уходит исходная строка." />
      <div className="live-actions"><Button type="submit" variant="filled" disabled={busy || !text.trim()}>{busy ? 'Разбираем адрес…' : 'Разобрать адрес'}</Button><span role="status" className="muted">{elapsed !== null ? `${elapsed.toLocaleString('ru-RU')} мс · полный HTTP-запрос` : busy ? 'Ожидаем ответ API' : 'Реальный запрос к API'}</span></div>
    </form>{error && <div role="alert" className="message error">{error} Повторите запрос.</div>}</section>
    {result ? <section className="panel"><ResultDetails result={result} /></section> : <div className="live-explanation"><h2>От строки к адресным полям</h2><ol><li>Нормализатор выделяет компоненты адреса.</li><li>Справочники КАТО/OSM уточняют структуру и выявляют неоднозначности.</li><li>Полнота полей и противоречия определяют необходимость проверки.</li></ol><p className="muted">Если данных недостаточно, результат требует оператора. Координаты не участвуют в разборе и оценке уверенности.</p></div>}
    </div></div>
  </main>
}
