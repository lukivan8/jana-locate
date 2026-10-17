import { useRef, useState } from 'react'
import dataset from '../../data/address-anomalies.json'
import datasetUrl from '../../data/address-anomalies.json?url'
import basesUrl from '../../data/address-bases.json?url'
import partOneUrl from '../../data/address-anomalies-part-1.json?url'
import partTwoUrl from '../../data/address-anomalies-part-2.json?url'
import { Button, SelectField, TextField } from '../../ui/material'
import type { InputRecord, Result } from '../../api/client'

const examples = dataset.records.map((record, index) => ({ ...record, ...dataset.scenarios[index] }))
const categoryLabels = new Map(dataset.categories.map(category => [category.id, category.label]))
const pageSize = 8
const resultFields = ['province', 'city', 'district', 'living_area', 'street_type', 'street_name', 'house', 'apartment', 'entrance', 'floor', 'landmark'] as const
const invisibleLabels: Record<string, string> = { '\u00a0': '[NBSP]', '\u200b': '[ZWSP]', '\ufeff': '[BOM]', '\u2060': '[WORD JOINER]', '\u202f': '[NNBSP]', '\t': '[TAB]', '\n': '[LF]', '\r': '[CR]' }
function visibleAddress(raw: string) {
  if (raw === '') return '[пустая строка]'
  if (raw.trim() === '') return `[пробелы: ${raw.length}]`
  return raw.replace(/[\u00a0\u200b\ufeff\u2060\u202f\t\n\r]/g, char => invisibleLabels[char])
}

export function AnomalyExamples({ busy, results, onNormalize }: { busy: boolean; results: Result[]; onNormalize: (records: InputRecord[]) => Promise<void> }) {
  const [category, setCategory] = useState('all')
  const [query, setQuery] = useState('')
  const [combinationsOnly, setCombinationsOnly] = useState(false)
  const [page, setPage] = useState(0)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const detail = useRef<HTMLElement>(null)
  const filtered = examples.filter(example => (category === 'all' || example.anomalies.includes(category)) && (!combinationsOnly || example.anomalies.length > 1) && `${example.id} ${visibleAddress(example.raw_address)} ${example.anomalies.map(tag => categoryLabels.get(tag)).join(' ')}`.toLocaleLowerCase().includes(query.toLocaleLowerCase().trim()))
  const visible = filtered.slice(page * pageSize, (page + 1) * pageSize)
  const selected = filtered.find(example => example.id === selectedId) ?? visible[0]
  const selectedIndex = filtered.findIndex(example => example.id === selected?.id)
  const result = results.length === 1 && results[0].id === selected?.id && results[0].raw_address === selected.raw_address ? results[0] : undefined
  function reset() { setPage(0); setSelectedId(null) }
  function turnPage(next: number) { setPage(next); setSelectedId(filtered[next * pageSize]?.id ?? null) }
  function choose(id: string) {
    setSelectedId(id)
    if (window.matchMedia('(max-width: 839px)').matches) requestAnimationFrame(() => detail.current?.scrollIntoView({ block: 'start' }))
  }
  function step(direction: number) {
    const next = selectedIndex + direction
    setPage(Math.floor(next / pageSize))
    choose(filtered[next].id)
  }

  return <section className="panel examples" aria-labelledby="examples-heading" lang="ru">
    <div className="section-title"><h2 id="examples-heading">Проверить на примерах</h2><a href={datasetUrl} download="address-anomalies.json">Скачать набор JSON</a></div>
    <p className="muted">{examples.length.toLocaleString('ru-RU')} сценариев без эталонной разметки. {dataset.handcrafted_count} придуманный пример и {dataset.source_based_count} изменённых вариантов {dataset.base_count} адресных записей OSM. Опечатки, транслит, детали доставки и комбинации искусственные; это набор для исследования, не оценка точности.</p>
    <p className="muted">Основа: <a href="https://www.openstreetmap.org/copyright">© OpenStreetMap contributors · ODbL</a>, снимок Казахстана. <a href={basesUrl} download="address-bases.json">Исходные записи и происхождение</a>.</p>
    <div className="actions"><a href={partOneUrl} download="address-anomalies-part-1.json">Скачать пакет 1 · 1000 адресов</a><a href={partTwoUrl} download="address-anomalies-part-2.json">Скачать пакет 2 · 1000 адресов</a></div>
    <p className="muted">Каждый пакет можно загрузить через основной мастер. Лимит API — 1000 записей на запрос.</p>
    <div className="example-filters">
      <SelectField label="Тип аномалии" value={category} onChange={event => { setCategory(event.target.value); reset() }}>
        <option value="all">Все типы</option>
        {dataset.categories.map(item => <option key={item.id} value={item.id}>{item.label}{item.source === 'hypothesis' ? ' · гипотеза' : item.source === 'requirements' ? ' · ТЗ' : ''}</option>)}
      </SelectField>
      <TextField label="Найти пример" type="search" value={query} onChange={event => { setQuery(event.target.value); reset() }} />
      <label className="example-checkbox"><input type="checkbox" checked={combinationsOnly} onChange={event => { setCombinationsOnly(event.target.checked); reset() }} />Только комбинации</label>
    </div>
    <p className="muted" aria-live="polite">Найдено: {filtered.length}{filtered.length > 0 && ` · ${page * pageSize + 1}–${Math.min((page + 1) * pageSize, filtered.length)}`}</p>
    <div className="example-layout">
      <div>
        <ul className="example-list" aria-label="Тестовые адреса">
          {visible.map(example => <li key={example.id}><button type="button" className="example-choice" aria-pressed={selected?.id === example.id} onClick={() => choose(example.id)}>
            <span className="example-address">{visibleAddress(example.raw_address)}</span>
            <span className="example-meta">{example.id} · {example.anomalies.map(tag => categoryLabels.get(tag)).join(' · ')}</span>
          </button></li>)}
        </ul>
        {!filtered.length && <p className="empty">Примеров не найдено. Измените поиск или фильтр.</p>}
        {filtered.length > pageSize && <nav className="example-pagination" aria-label="Страницы примеров">
          <Button onClick={() => turnPage(page - 1)} disabled={page === 0}>Назад</Button>
          <span>{page + 1} / {Math.ceil(filtered.length / pageSize)}</span>
          <Button onClick={() => turnPage(page + 1)} disabled={(page + 1) * pageSize >= filtered.length}>Далее</Button>
        </nav>}
      </div>
      {selected && <section ref={detail} className="example-detail" aria-labelledby="example-detail-heading" aria-busy={busy}>
        <h3 id="example-detail-heading">Выбранный адрес</h3>
        <p className="example-meta">{selected.id} · {selected.anomalies.map(tag => categoryLabels.get(tag)).join(' · ')}</p>
        <pre className="example-raw" role="group" aria-label="Исходный тестовый адрес">{visibleAddress(selected.raw_address)}</pre>
        <p className="muted">{selected.note}</p>
        {'source_url' in selected && typeof selected.source_url === 'string' && <p className="muted"><a href={selected.source_url} target="_blank" rel="noreferrer">Объект OSM, использованный как основа</a>. Изменённая строка не является проверенным адресом доставки.</p>}
        <p className="muted">Обозначения в квадратных скобках показывают скрытые символы. В обработку уходит исходная строка, включая пробелы и переносы.</p>
        <Button variant="filled" disabled={busy} onClick={() => onNormalize([{ id: selected.id, raw_address: selected.raw_address }])}>{busy ? 'Обработка…' : 'Нормализовать адрес'}</Button>
        <nav className="example-pagination" aria-label="Переключение адресов">
          <Button aria-label="Предыдущий адрес" onClick={() => step(-1)} disabled={selectedIndex <= 0}>Предыдущий</Button>
          <Button aria-label="Следующий адрес" onClick={() => step(1)} disabled={selectedIndex >= filtered.length - 1}>Следующий</Button>
        </nav>
        {result ? <section className="example-result" aria-label="Результат тестового адреса" aria-live="polite">
          <h3>Результат разбора</h3>
          <p className="muted">Уверенность: {Math.round(result.confidence * 100)}% · {result.status.replaceAll('_', ' ')}</p>
          <dl>{resultFields.map(field => <div key={field}><dt>{field}</dt><dd>{result[field] ?? <span className="muted">null</span>}</dd></div>)}</dl>
          {result.warnings.length > 0 && <p className="muted">Предупреждения: {result.warnings.join(' · ')}</p>}
        </section> : <p className="muted">Нажмите «Нормализовать адрес», чтобы увидеть поля здесь. Результат появится под выбранным примером.</p>}
      </section>}
    </div>
  </section>
}
