import { useState } from 'react'
import { normalize, type Result } from '../../api/client'
import { Button, TextArea } from '../../ui/material'
import { CoordinateMap } from '../addresses/coordinate-map'
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
  return <main id="main"><div className="intro"><div><h1>Любой адрес. Разбор на ваших глазах.</h1><p>Введите свой адрес с сокращениями, опечатками или деталями доставки. Сервис разберёт строку и попробует найти здание в каталоге.</p></div></div>
    <div className="live-layout"><div><section className="panel"><form onSubmit={e => { e.preventDefault(); void run() }} aria-busy={busy}>
      <TextArea label="Адрес в свободной форме" value={text} onChange={e => { setText(e.target.value); setResult(null); setElapsed(null); setError('') }} disabled={busy} rows={4} placeholder="Например: Астана, Туран 55/6, кв 12" supportingText="Один адрес. В обработку уходит исходная строка." />
      <div className="live-actions"><Button type="submit" variant="filled" disabled={busy || !text.trim()}>{busy ? 'Разбираем адрес…' : 'Найти на карте'}</Button><span role="status" className="muted">{elapsed !== null ? `${elapsed.toLocaleString('ru-RU')} мс · полный HTTP-запрос` : busy ? 'Ожидаем ответ API' : 'Реальный запрос к API'}</span></div>
    </form>{error && <div role="alert" className="message error">{error} Повторите запрос.</div>}</section>
    {result ? <section className="panel"><ResultDetails result={result} /></section> : <div className="live-explanation"><h2>От строки к координатам</h2><ol><li>libpostal и правила выделяют компоненты адреса.</li><li>GLiNER дополняет разбор; справочники разрешают противоречия.</li><li>Каталог возвращает координаты известного здания.</li></ol><p className="muted">Если данных недостаточно, результат требует оператора. Новый ввод не гарантирует наличие здания в каталоге.</p></div>}
    </div><section className="panel map-panel"><div className="section-title"><h2>Местоположение здания</h2><span>{result?.coordinates ? 'Координаты получены' : 'Ожидание координат'}</span></div><CoordinateMap results={result ? [result] : []} />{result && result.status !== 'matched' && <p className="review-callout">Нужна проверка оператора. {result.coordinates ? 'Проверьте адрес и источник координат.' : 'Не удалось автоматически подтвердить здание.'}</p>}</section></div>
  </main>
}
