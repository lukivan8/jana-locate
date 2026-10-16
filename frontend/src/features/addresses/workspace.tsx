import { useEffect, useState } from 'react'
import { Button, TextArea, TextField } from '../../ui/material'
import { catalog, confirm, normalize, type CatalogEntry, type Fields, type InputRecord, type Result } from '../../api/client'
import { AnomalyExamples } from './anomaly-examples'
import { CoordinateMap } from './coordinate-map'
import { fieldLabels, statusLabels, warningLabel, ResultDetails } from './result-details'

const demo = 'Ул .Курильская 9 кв 1\nТуран 55/6 магазин Береке 4подьезд 9этаж\n66квартал 12дом 69квартира\nНеизвестный адрес'
const fieldNames: (keyof Fields)[] = ['province', 'city', 'district', 'living_area', 'street_type', 'street_name', 'house', 'apartment', 'entrance', 'floor', 'landmark']
const buildingFields: (keyof Fields)[] = ['province', 'city', 'district', 'living_area', 'street_type', 'street_name', 'house']
const empty: Fields = { province: null, city: null, district: null, living_area: null, street_type: null, street_name: null, house: null, apartment: null, entrance: null, floor: null, landmark: null }
const identity = (f: Fields) => [f.city, f.street_type, f.street_name, f.house].map(v => v?.trim().toLowerCase().replace(/\s+/g, ' ') ?? '').join('|')
function readRecords(text: string): InputRecord[] {
  const value: unknown = JSON.parse(text)
  const records = Array.isArray(value) ? value : (value as { records?: unknown })?.records
  if (!Array.isArray(records) || records.length < 1 || records.length > 1000) throw new Error('Загрузите массив или {records: [...]} с 1–1 000 записями.')
  for (const r of records) {
    if (!r || typeof r !== 'object' || typeof r.raw_address !== 'string' || !(typeof r.id === 'string' || (typeof r.id === 'number' && Number.isSafeInteger(r.id)))) throw new Error('Каждой записи нужны id (строка или целое число) и raw_address (строка). Большие ID передавайте строками.')
  }
  return records as InputRecord[]
}
export function AddressWorkspace() {
  const [text, setText] = useState(demo)
  const [results, setResults] = useState<Result[]>([])
  const [entries, setEntries] = useState<CatalogEntry[]>([])
  const [selected, setSelected] = useState<number | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [reviewOnly, setReviewOnly] = useState(false)
  const [upload, setUpload] = useState<InputRecord[] | null>(null)
  const [draft, setDraft] = useState<Fields>(empty)
  const [lat, setLat] = useState('')
  const [lon, setLon] = useState('')
  const [revision, setRevision] = useState(0)
  const [loadedIdentity, setLoadedIdentity] = useState('')
  const [elapsed, setElapsed] = useState<number | null>(null)
  useEffect(() => { if (selected !== null) document.getElementById('correction-heading')?.focus() }, [selected])
  useEffect(() => { let active = true; catalog().then(data => { if (active) setEntries(data) }).catch(e => { if (active) setError(e.message) }); return () => { active = false } }, [])
  function select(index: number) {
    const r = results[index], fields = Object.fromEntries(fieldNames.map(key => [key, r[key]])) as Fields
    setSelected(index); setDraft(fields); setLat(String(r.coordinates?.latitude ?? '')); setLon(String(r.coordinates?.longitude ?? '')); setRevision(r.catalog_revision ?? 0); setLoadedIdentity(identity(fields)); setNotice('')
  }
  async function runRecords(records: InputRecord[]) {
    setBusy(true); setError(''); setNotice(''); setSelected(null)
    setReviewOnly(false)
    setResults([]); setElapsed(null)
    const started = performance.now()
    try { if (!records.length || records.length > 1000) throw new Error('Введите от 1 до 1 000 адресов.'); const [parsed, buildings] = await Promise.all([normalize(records), catalog()]); setResults(parsed); setEntries(buildings); setElapsed(Math.round(performance.now() - started)) }
    catch (e) { setError(e instanceof Error ? e.message : String(e)) }
    finally { setBusy(false) }
  }
  async function run() {
    await runRecords(upload ?? text.split('\n').filter(line => line.trim()).map((raw_address, i) => ({ id: String(i + 1), raw_address })))
  }
  async function save() {
    setBusy(true); setError(''); setNotice('')
    try {
      if (lat.trim() === '' || lon.trim() === '' || !Number.isFinite(Number(lat)) || !Number.isFinite(Number(lon))) throw new Error('Укажите широту и долготу после проверки местоположения здания.')
      const fields = { ...empty, ...Object.fromEntries(buildingFields.map(key => [key, draft[key]])) }
      const changed = identity(fields) !== loadedIdentity
      if (changed && entries.some(e => identity(e.fields) === identity(fields))) throw new Error('Здание уже есть в каталоге. Нормализуйте его адрес и выберите результат перед редактированием.')
      const saved = await confirm(fields, Number(lat), Number(lon), changed ? 0 : revision)
      const refreshed = await normalize(results.map(r => ({ id: r.id, raw_address: r.raw_address })))
      // Preserve reviewed fields in the export. The durable catalog contains building fields only.
      if (selected !== null) refreshed[selected] = { ...refreshed[selected], ...draft, coordinates: saved.coordinates, catalog_id: saved.id, catalog_revision: saved.revision, status: 'matched', warnings: ['manually_reviewed'] }
      setResults(refreshed); setEntries(await catalog()); setSelected(null); setNotice('Здание сохранено. Проверенные поля включены в экспорт этой сессии.')
    } catch (e) { setError(e instanceof Error ? e.message : String(e)) }
    finally { setBusy(false) }
  }
  function download() {
    const url = URL.createObjectURL(new Blob([JSON.stringify({ results }, null, 2)], { type: 'application/json' }))
    const a = document.createElement('a'); a.href = url; a.download = 'jana-address-results.json'; a.click(); URL.revokeObjectURL(url)
  }
  const needsReview = results.filter(r => r.status !== 'matched').length
  return <main id="main">
    <div className="intro"><div><h1>Пакет адресов → очередь решений.</h1><p>Загрузите адреса, сравните исходные строки с результатом разбора и передайте неопределённые случаи оператору.</p></div><div className="catalog-summary"><strong>{entries.length}</strong> зданий в каталоге<br /><span>{entries.filter(e => e.confirmed).length} подтверждено оператором</span></div></div>
    <aside className="demo-note">Демо-каталог содержит три синтетических здания. Их координаты иллюстративные и требуют проверки. Точность на реальных данных не измерена.</aside>
    {error && <div role="alert" className="message error">{error}</div>}{notice && <div role="status" className="message">{notice}</div>}
    <div className="batch-layout"><section className="panel input"><div className="section-title"><h2>Загрузить адреса</h2><span>1–1 000 записей</span></div>
      <TextArea label="Один адрес на строку" id="addresses" value={text} disabled={busy || !!upload} onChange={e => setText(e.target.value)} rows={6} />
      <div className="upload"><label htmlFor="file">Загрузить JSON</label><input id="file" type="file" accept=".json,application/json" disabled={busy} onChange={async e => { const f = e.target.files?.[0]; if (!f) return; try { if (f.size > 2 * 1024 * 1024) throw new Error('Файл превышает 2 MiB.'); setUpload(readRecords(await f.text())); setError('') } catch (err) { setUpload(null); setError(err instanceof Error ? err.message : String(err)) } }} />
      {upload ? <p role="status">Загружено записей: {upload.length} <Button variant="text" disabled={busy} onClick={() => { setUpload(null); (document.getElementById('file') as HTMLInputElement).value = '' }}>Использовать текст</Button></p> : <p className="muted">JSON: массив или {'{ records: [...] }'}. Поля: id и raw_address. До 2 MiB.</p>}</div>
      <Button variant="filled" disabled={busy} onClick={run}>{busy ? 'Обработка…' : 'Нормализовать пакет'}</Button>
    </section><section className="batch-pipeline" aria-label="Пайплайн обработки"><h2>Как обрабатывается адрес</h2><ol><li><strong>Разбор строки</strong><span>libpostal + правила выделяют компоненты</span></li><li><strong>Согласование</strong><span>GLiNER + справочники уточняют структуру</span></li><li><strong>Поиск здания</strong><span>Каталог связывает адрес с координатами</span></li><li><strong>Решение оператора</strong><span>Неопределённые поля и конфликты остаются видимыми</span></li></ol><p className="muted">Недоступность моделей отражается в предупреждениях. Автоматический разбор не равен подтверждённой геопозиции.</p></section></div>
    <section className="panel results" aria-busy={busy}><div className="section-title"><h2>Очередь проверки</h2><div className="actions"><label><input type="checkbox" checked={reviewOnly} disabled={busy} onChange={e => setReviewOnly(e.target.checked)} /> Только требующие проверки</label><Button disabled={!results.length || busy} onClick={download}>Экспорт JSON</Button></div></div>
      {results.length > 0 && <div className="batch-outcome" role="status"><span>Обработано: <strong>{results.length}</strong></span><span>Найдено в каталоге: <strong>{results.length - needsReview}</strong></span><span>Требуют оператора: <strong>{needsReview}</strong></span><span>{elapsed?.toLocaleString('ru-RU')} мс · HTTP + каталог</span></div>}
      {needsReview > 0 && <p className="review-callout">{needsReview} из {results.length} адресов требуют вмешательства оператора. Проверьте поля и координаты перед подтверждением здания.</p>}
      {results.length === 0 ? <p className="empty">{busy ? 'Нормализуем адреса. Результаты появятся после ответа API.' : 'Загрузите JSON или запустите пакет из поля выше.'}</p> : <div className="table-scroll" role="region" aria-label="Результаты проверки адресов" tabIndex={0}><p className="muted table-hint">Таблица прокручивается по горизонтали. Справа — статус и проверка.</p><table><thead><tr><th>Исходный адрес</th><th>Нормализованное здание</th><th>Уверенность</th><th>Решение</th><th>Действие</th></tr></thead><tbody>{results.map((r, i) => reviewOnly && r.status === 'matched' ? null : <tr key={i} className={r.status !== 'matched' ? 'requires-review' : ''}><td>{r.raw_address || <em>Пустая строка</em>}<small>ID {String(r.id)}</small></td><td>{[r.city, r.street_type, r.street_name, r.house].filter(Boolean).join(', ') || 'Не определено'}<small>{r.warnings.map(warningLabel).join(' · ')}</small></td><td>{Math.round(r.confidence * 100)}%</td><td><span className={`badge ${r.status}`}>{statusLabels[r.status]}</span><small>{r.status !== 'matched' ? 'Требуется оператор' : 'Здание определено'}</small></td><td><Button disabled={busy} onClick={() => select(i)}>{r.status === 'matched' ? 'Просмотреть' : 'Проверить'}</Button></td></tr>)}</tbody></table>{reviewOnly && !needsReview && <p className="empty">Все адреса обработаны. Очередь проверки пуста.</p>}</div>}
    </section>
    {selected !== null && <section className="panel editor" aria-label="Исправление адреса"><div className="section-title"><h2 id="correction-heading" tabIndex={-1}>Проверить и подтвердить</h2><Button onClick={() => setSelected(null)} disabled={busy}>Закрыть</Button></div><p className="raw-selected">{results[selected].raw_address}</p><ResultDetails result={results[selected]} /><div className="field-grid">{fieldNames.map(key => <TextField key={key} label={fieldLabels[key]} value={draft[key] ?? ''} disabled={busy} onChange={e => setDraft({ ...draft, [key]: e.target.value || null })} />)}<TextField label="Широта" type="number" step="any" min="-90" max="90" value={lat} disabled={busy} onChange={e => setLat(e.target.value)} /><TextField label="Долгота" type="number" step="any" min="-180" max="180" value={lon} disabled={busy} onChange={e => setLon(e.target.value)} /></div><p className="muted">Подтвердите координаты по проверенному источнику. Сохраняются только поля здания. Детали доставки остаются в экспорте сессии. При конфликте ревизий запустите пакет заново.</p><Button variant="filled" onClick={save} disabled={busy}>Подтвердить здание и сохранить</Button></section>}
    <details className="batch-map"><summary>Карта результатов · {results.filter(r => r.coordinates).length} с координатами</summary><CoordinateMap results={results} onSelect={select} /></details>
    <details className="example-disclosure"><summary>Исследовать тестовые примеры</summary><AnomalyExamples busy={busy} results={results} onNormalize={runRecords} /></details>
  </main>
}
