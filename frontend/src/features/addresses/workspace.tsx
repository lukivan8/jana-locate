import { nextReview } from './review-navigation'
import { PublishedWorkspace } from './published'
import { RunTiming } from './run-timing'
import { Fragment, useEffect, useRef, useState } from 'react'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { Button, CircularProgress, LinearProgress, TextArea, TextField } from '../../ui/material'
import { createJob, getJob, reviewRecord, isLocalWorkspace, importLocalSource, localRunAction, localExportUrl, subscribeLocalRuns, runSummary, type RunSummary, type LocalRun, type LocalSource, type Job, type Fields, type InputRecord, type Result } from '../../api/client'
import { addressLines, readAddressFile } from './import-addresses'
import { ImportColumnDialog } from './import-column-dialog'
import { CityCandidatePicker } from './city-candidates'
import { confidenceLabel, fieldLabels, displayFieldValue, isReviewed, ReviewStatus, ResultWarnings, ResultDetails } from './result-details'

// Synthetic inputs checked against the normalizer; no expected answers or coordinates.
const exampleAddresses = [
  'Көкшетау, Әуезов 15 кв6, 2подъезд 3этаж, магазин Береке',
  'Астана, Үркер, ул.Дулат Бабатайұлы 15 кв6, 2подъезд 3этаж, магазин Береке',
  'Алматы, Алмалинский район, ул.Курильская 9 кв1, 3подъезд 2этаж, магазин Береке',
  'Almaty, Kurilskaya 9 kv1',
  'Pavlodar, Lenina 18 kv9',
  'Астана, Kenesary 20 kv4',
  'кв1, 3подъезд 2этаж, ул.Курильская 9, Алматы, магазин Береке',
  'ул.Кенесары 20 кв4, Астана, район Сарыарка, 2подъезд 5этаж',
  'ул.Шугаева 15кв7, Семей, 4подъезд 9этаж, аптека Айболит',
  'Астана, Пригородный, ул.Арнасай 18 кв9, 2подьезд 3этаж, звонить за 10 минут',
].join('\n')

const fieldNames: (keyof Fields)[] = ['province', 'city', 'district', 'living_area', 'street_type', 'street_name', 'house', 'apartment', 'entrance', 'floor', 'landmark']
const empty: Fields = { province: null, city: null, district: null, living_area: null, street_type: null, street_name: null, house: null, apartment: null, entrance: null, floor: null, landmark: null }
const localLaunchModes = [
  { id: 'api', label: 'API' },
  { id: 'web', label: 'API + Web' },
  { id: 'full', label: 'API + Web + CLI' },
] as const

function LocalLaunch() {
  const [mode, setMode] = useState<(typeof localLaunchModes)[number]>(localLaunchModes[2])
  const [copyStatus, setCopyStatus] = useState('')
  const command = `curl -fsSL https://raw.githubusercontent.com/lukivan8/jana-locate/main/scripts/install.sh | sh -s -- ${mode.id}`
  async function copyCommand() {
    try {
      await navigator.clipboard.writeText(command)
      setCopyStatus('Скопировано')
    } catch {
      setCopyStatus('Скопируйте команду вручную')
    }
  }
  return <section className="local-launch" aria-labelledby="local-launch-title">
    <div className="local-launch-heading"><h2 id="local-launch-title">Запустить локально</h2><span>Docker</span></div>
    <div className="local-launch-terminal">
      <span className="local-launch-prompt" aria-hidden="true">$</span>
      <pre className="local-launch-command" tabIndex={0} aria-label="Команда установки"><code>{command}</code></pre>
      <Button className="local-launch-copy" variant="text" aria-label="Копировать команду" title="Копировать команду" onClick={() => { void copyCommand() }}>
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true"><rect x="8" y="8" width="12" height="13" rx="2" /><path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3" /></svg>
      </Button>
    </div>
    <div className="local-launch-footer">
      <div className="local-launch-modes" role="group" aria-label="Скачать комплект">
        {localLaunchModes.map(option => <Button key={option.id} variant={mode.id === option.id ? 'tonal' : 'outlined'} aria-pressed={mode.id === option.id} onClick={() => { setMode(option); setCopyStatus('') }}>{option.label}</Button>)}
      </div>
      <span className="local-launch-hint" role="status">{copyStatus || 'Готовые контейнеры · без сборки исходников'}</span>
    </div>
  </section>
}

export function AddressWorkspace() {
  const local = isLocalWorkspace()
  const [localRuns, setLocalRuns] = useState<LocalRun[]>([])
  const [, setStreamConnected] = useState(false)
  const [offset, setOffset] = useState(0)
  const [source, setSource] = useState<LocalSource>({ schema: 'public', table: '', key: 'id', column: 'address', target_schema: 'public', target_table: '' })
  const { job: jobId, new: newRun } = useSearch({ from: '/' })
  const navigate = useNavigate({ from: '/' })
  const [job, setJob] = useState<Job | null>(null)
  const [loadAttempt, setLoadAttempt] = useState(0)
  const [loadingJob, setLoadingJob] = useState(false)
  const [dirty, setDirty] = useState(false)
  const [text, setText] = useState('')
  const [results, setResults] = useState<Result[]>([])
  const [selected, setSelected] = useState<number | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [reviewOnly, setReviewOnly] = useState(false)
  const [upload, setUpload] = useState<InputRecord[] | null>(null)
  const [pendingImport, setPendingImport] = useState<{ rows: string[][]; name: string; sheetName?: string } | null>(null)
  const [readingFile, setReadingFile] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const wizard = useRef<HTMLDialogElement>(null)
  const adoptedPage = useRef<{ id: string; offset: number; reviewOnly: boolean } | null>(null)
  const published = job?.local?.run.status === 'published' && job.local.run.source.kind !== 'file'
  const [summary, setSummary] = useState<RunSummary | null>(null)
  useEffect(() => {
    if (!job?.local) return
    let active = true
    void runSummary(job.id).then(value => { if (active) setSummary(value) }).catch(() => { if (active) setSummary(null) })
    return () => { active = false }
  }, [job?.id, job?.revision])
  useEffect(() => {
    const dialog = wizard.current
    if (!dialog) return
    dialog.close()
    if (expanded) dialog.showModal()
    else dialog.show()
    if (expanded) document.getElementById('addresses')?.focus()
    return () => { dialog.close() }
  }, [expanded, published])
  useEffect(() => {
    if (!expanded) return
    const previous = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => { document.body.style.overflow = previous }
  }, [expanded])
  const [draft, setDraft] = useState<Fields>(empty)
  const [elapsed, setElapsed] = useState<number | null>(null)
  const working = busy || loadingJob || (!error && (job?.status === 'queued' || job?.status === 'running'))
  function adopt(saved: Job) { setJob(saved); setResults(saved.results); setDirty(false) }
  useEffect(() => {
    if (!jobId) { setJob(null); setResults([]); setSelected(null); setText(''); setUpload(null); setDirty(false); setError(''); setLoadingJob(false); return }
    if (adoptedPage.current?.id === jobId && adoptedPage.current.offset === offset && adoptedPage.current.reviewOnly === reviewOnly) {
      adoptedPage.current = null
      return
    }
    let active = true
    let timer: ReturnType<typeof setTimeout>
    setLoadingJob(true); setJob(null); setResults([]); setSelected(null); setError('')
    async function load() {
      try {
        const saved = await getJob(jobId!, { offset, reviewOnly })
        if (!active) return
        adopt(saved); setUpload(saved.records); setText(addressLines(saved.records)); setElapsed(null)
        setLoadingJob(false)
        if (saved.status === 'queued' || saved.status === 'running') { if (!local) timer = setTimeout(load, 1000) }
        else { if (saved.local?.run.status !== 'published' && saved.total === 1 && saved.results.length === 1) editResult(0, saved.results[0]); if (saved.status === 'failed') setError(saved.error) }
      } catch (e) { if (active) { setError(e instanceof Error ? e.message : String(e)); setLoadingJob(false) } }
    }
    void load()
    return () => { active = false; clearTimeout(timer) }
  }, [jobId, loadAttempt, local ? offset : 0, local ? reviewOnly : false])
  useEffect(() => {
    if (!local) return
    return subscribeLocalRuns(setLocalRuns, setStreamConnected)
  }, [local])
  useEffect(() => {
    if (!local || jobId || newRun || !localRuns.length) return
    void navigate({ search: { job: localRuns[0].id }, replace: true })
  }, [local, jobId, newRun, localRuns, navigate])
  useEffect(() => {
    const latest = localRuns.find(run => run.id === jobId)
    // An open editor retains its original row revision so concurrent saves can
    // conflict explicitly; a stream update must never replace an operator draft.
    if (!local || !jobId || !job?.local || !latest || latest.revision <= job.revision || ((dirty || selected !== null) && latest.status !== 'published') || busy) return
    let active = true
    void getJob(jobId, { offset, reviewOnly }).then(saved => {
      if (!active) return
      setJob(saved); setResults(saved.results); setUpload(saved.records); setText(addressLines(saved.records)); if (saved.local?.run.status === 'published') { setSelected(null); setDirty(false) }
      if (saved.status === 'failed') setError(saved.error)
      if (saved.local?.run.status !== 'published' && saved.status === 'completed' && saved.total === 1 && saved.results.length === 1) editResult(0, saved.results[0])
    }).catch(e => { if (active) setError(e instanceof Error ? e.message : String(e)) })
    return () => { active = false }
  }, [local, localRuns, jobId, job?.revision, !!job?.local, offset, reviewOnly, dirty, selected, busy])
  useEffect(() => {
    if (!dirty) return
    const warn = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = '' }
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [dirty])
  useEffect(() => { if (error) document.getElementById('workspace-error')?.focus() }, [error])
  function leaveDraft() { return !dirty || window.confirm('Есть несохранённые изменения. Продолжить без сохранения?') }
  async function openSaved(id: string) {
    if (!leaveDraft()) return
    setDirty(false); setSelected(null); setOffset(0)
    if (id === jobId) setLoadAttempt(value => value + 1)
    else await navigate({ search: { job: id } })
  }
  useEffect(() => { if (selected !== null) document.getElementById('correction-heading')?.focus() }, [selected, job?.local?.rows[selected ?? -1]?.position])
  function select(index: number) {
    if (!leaveDraft()) return
    setDirty(false)
    if (selected === index) { setSelected(null); return }
    editResult(index, results[index])
  }
  function editResult(index: number, r: Result) {
    const fields = Object.fromEntries(fieldNames.map(key => [key, r[key]])) as Fields
    setSelected(index); setDraft(fields); }
  async function runRecords(records: InputRecord[]) {
    if (!leaveDraft()) return
    setBusy(true); setError(''); setSelected(null); setDirty(false)
    setReviewOnly(false); setOffset(0)
    const started = performance.now()
    try {
      if (!records.length || records.length > 1000) throw new Error('Введите от 1 до 1 000 адресов.')
      const saved = await createJob(records)
      adopt(saved); setElapsed(Math.round(performance.now() - started))
      await navigate({ search: { job: saved.id } })
    } catch (e) { setError(e instanceof Error ? e.message : String(e)) }
    finally { setBusy(false) }
  }

  function applyImport(records: InputRecord[]) {
    setUpload(records); setText(addressLines(records)); setError(''); setPendingImport(null)
  }
  async function run() {
    setExpanded(false)
    const records = upload && text === addressLines(upload) ? upload : text.split('\n').flatMap((raw_address, i) => raw_address.trim() ? [{ id: upload?.[i]?.id ?? String(i + 1), raw_address }] : [])
    await runRecords(records)
  }
  async function reset() {
    if (!leaveDraft()) return
    setDirty(false); setText(''); setUpload(null); setResults([]); setJob(null); setSelected(null); setError(''); setReviewOnly(false); setOffset(0)
    await navigate({ search: local ? { new: true } : {} })
    document.getElementById('addresses')?.focus()
  }
  async function saveFields() {
    if (!job || selected === null) return
    setBusy(true); setError('')
    try {
      const position = job.local?.rows[selected].position ?? selected
      const saved = await reviewRecord(job, selected, draft)
      adopt(saved); setSelected(null)
      if (saved.local) { setUpload(saved.records); setText(addressLines(saved.records)) }
      try {
        const next = await nextReview(saved, position)
        if (next) {
          if (next.job.local) {
            const page = next.job.local
            if (page.offset !== offset) adoptedPage.current = { id: saved.id, offset: page.offset, reviewOnly }
            setOffset(page.offset); setUpload(next.job.records); setText(addressLines(next.job.records))
          }
          adopt(next.job); editResult(next.index, next.job.results[next.index])
        }
      } catch (e) { setError(`Адрес подтверждён, но следующий адрес загрузить не удалось. ${e instanceof Error ? e.message : String(e)}`) }
    } catch (e) { setError(e instanceof Error ? e.message : String(e)) }
    finally { setBusy(false) }
  }
  async function importSource() {
    if (!leaveDraft()) return
    setBusy(true); setError(''); try { const id = await importLocalSource({ ...source, target_table: source.target_table || `${source.table}_jana_addresses` }); setOffset(0); setReviewOnly(false); await navigate({ search: { job: id } }) }
    catch (e) { setError(e instanceof Error ? e.message : String(e)) }
    finally { setBusy(false) }
  }
  async function runAction(action: 'resume' | 'publish') {
    if (!job || !leaveDraft()) return
    setBusy(true); setError('')
    try {
      if (action === 'publish') {
        const currentSummary = await runSummary(job.id)
        if (!window.confirm(`Записать ${job.total} результатов в ${job.local?.run.source.target_schema}.${job.local?.run.source.target_table}? Требует проверки: ${currentSummary.needs_review}. Исходная таблица останется без изменений.`)) return
      }
      adopt(await localRunAction(job, action)); setSelected(null); if (action === 'resume') setLoadAttempt(value => value + 1) }
    catch (e) { setError(e instanceof Error ? e.message : String(e)) }
    finally { setBusy(false) }
  }
  function download() {
    if (job?.local) { const a = document.createElement('a'); a.href = localExportUrl(job); a.click(); return }
    const url = URL.createObjectURL(new Blob([JSON.stringify({ results }, null, 2)], { type: 'application/json' }))
    const a = document.createElement('a'); a.href = url; a.download = 'jana-address-results.json'; a.click(); URL.revokeObjectURL(url)
  }
  const needsReview = results.filter((r, i) => r.status !== 'matched' && !isReviewed(r, job?.reviewed[i])).length
  if (published && job?.local) return <PublishedWorkspace key={job.id} run={job.local.run} onNewRun={() => { void reset() }} />
  return <main id="main" className="address-workspace">
    <dialog ref={wizard} className={`panel address-wizard${expanded ? ' address-wizard--expanded' : ''}`} aria-labelledby="wizard-heading" onCancel={event => { event.preventDefault(); setExpanded(false); document.getElementById('expand-wizard')?.focus() }}>
      <div className="section-title"><h1 id="wizard-heading">Нормализация адресов</h1><Button id="expand-wizard" className="icon-button" variant="text" aria-label={expanded ? 'Свернуть' : 'На весь экран'} title={expanded ? 'Свернуть' : 'На весь экран'} aria-expanded={expanded} onClick={() => setExpanded(value => !value)}><svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d={expanded ? 'M4 9h5V4M15 4v5h5M20 15h-5v5M9 20v-5H4' : 'M9 4H4v5M15 4h5v5M20 15v5h-5M9 20H4v-5'} /></svg></Button></div>
      <TextArea label="Один адрес на строку" id="addresses" value={text} disabled={working || readingFile} readOnly={!!job?.local} onChange={e => setText(e.target.value)} rows={5} placeholder="Например: Караганда, улица Алалыкина 12" />
      {!job?.local && <button type="button" className="wizard-examples" disabled={working || readingFile} title="10 синтетических примеров: казахские написания, транслит, разный порядок, районы, жилые массивы, ориентиры, подъезды и этажи" onClick={() => { setUpload(null); setText(exampleAddresses); setError(''); document.getElementById('addresses')?.focus() }}>Подставить примеры</button>}
      <div className="wizard-meta"><span>{job?.local ? `${job.local.run.processed} / ${job.total} адресов` : `${text.split('\n').filter(line => line.trim()).length} / 1 000 адресов`}</span>{!job?.local && <span>Проверьте список перед запуском</span>}</div>
      <div className="wizard-actions"><Button variant="filled" disabled={working || readingFile || !!job?.local || !text.trim()} onClick={run}>{working && <CircularProgress compact />}{working ? 'Обработка…' : 'Нормализовать пакет'}</Button>
        <div className="wizard-upload"><label className={`md-button md-button--text${working || readingFile ? ' upload-disabled' : ''}`} htmlFor="file">Загрузить файл<input id="file" aria-label="Загрузить JSON, TXT, CSV или XLSX" type="file" accept=".txt,.json,.csv,.xlsx,text/plain,application/json,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" disabled={working || readingFile || !!job?.local} onChange={async e => {
          const file = e.target.files?.[0]; e.target.value = ''; if (!file) return
          setReadingFile(true)
          try {
            const imported = await readAddressFile(file)
            if ('records' in imported) applyImport(imported.records)
            else { setError(''); setPendingImport({ ...imported, name: file.name }) }
          }
          catch (err) { setError(err instanceof Error ? err.message : String(err)) }
          finally { setReadingFile(false) }
        }} /></label><span>{readingFile ? 'Читаем файл…' : 'TXT, JSON, CSV, XLSX · до 2 MiB'}</span></div>
      </div>
      {error && <div id="workspace-error" tabIndex={-1} role="alert" className="message error">{error}</div>}{error && jobId && <Button disabled={working} onClick={() => { void openSaved(jobId) }}>Загрузить сохранённую версию</Button>}
      {local && !job && <details className="source-import"><summary>Прочитать колонку PostgreSQL</summary><p className="muted">CLI сохранит локальный снимок. Исходная таблица останется без изменений. До 100 000 строк / 64 MiB.</p><form onSubmit={event => { event.preventDefault(); void importSource() }}><div className="field-grid">{(Object.keys(source) as (keyof LocalSource)[]).map(key => <TextField key={key} label={({ schema: 'Схема', table: 'Таблица', key: 'Первичный ключ', column: 'Колонка адреса', target_schema: 'Схема результата', target_table: 'Таблица результата' })[key]} value={source[key]} required={key !== 'target_table'} maxLength={63} disabled={working} onChange={event => setSource({ ...source, [key]: event.target.value })} />)}</div><Button variant="filled" type="submit" disabled={working}>Прочитать и нормализовать</Button></form></details>}
    </dialog>
    {pendingImport && <ImportColumnDialog {...pendingImport} onImport={applyImport} onCancel={() => setPendingImport(null)} />}
    {(jobId || job) && <div className="workspace-reset"><Button variant="text" disabled={working} onClick={() => { void reset() }}>Сбросить</Button></div>}
    {(results.length > 0 || working || jobId) && <section className="panel results" aria-busy={working}><div className="section-title"><h2>Очередь проверки</h2><div className="actions"><label><input type="checkbox" checked={reviewOnly} disabled={working} onChange={e => { if (!leaveDraft()) return; setSelected(null); setDirty(false); setOffset(0); setReviewOnly(e.target.checked) }} /> Только требующие проверки</label>{job?.local && job.local.run.source.kind !== 'file' ? <Button variant="filled" disabled={working || job.local.run.status !== 'review'} onClick={() => { void runAction('publish') }}>Опубликовать {job.local.run.source.target_table}</Button> : <Button disabled={!results.length || working || job?.local?.run.status === 'failed'} onClick={download}>Экспорт JSON</Button>}</div></div>
      {job?.local && <div className="local-run-info"><p id="run-source">{job.local.run.source.kind === 'file' ? `Файл: ${job.local.run.source.file_name ?? job.local.run.source.table}` : `Источник: ${job.local.run.source.schema}.${job.local.run.source.table}.${job.local.run.source.column}`}</p>{job.local.run.status === 'normalizing' && <LinearProgress id="progress" label="Обработка адресов" value={job.local.run.processed} max={job.total} />}{job.local.run.status === 'failed' && <Button disabled={working} onClick={() => { void runAction('resume') }}>Возобновить обработку</Button>}</div>}
      {job?.local && <dl className="run-counts" aria-label="Итоги запуска" role="status"><div><dt>Всего записей</dt><dd>{job.total}</dd></div><div><dt>Автоматически</dt><dd>{summary?.automatic ?? '…'}</dd></div><div><dt>Требуют проверки</dt><dd>{summary?.needs_review ?? '…'}</dd></div></dl>}
      {job?.local && <RunTiming run={job.local.run} />}
      {!local && results.length > 0 && <div className="batch-outcome" role="status"><span>Обработано: <strong>{results.length}</strong></span><span>Нормализовано: <strong>{results.filter(r => r.status === 'matched').length}</strong></span><span>Требуют оператора: <strong>{needsReview}</strong></span>{elapsed !== null && <span>{elapsed.toLocaleString('ru-RU')} мс · создание задания</span>}</div>}
      {results.length === 0 ? working ? <div className="normalization-loading" role="status"><CircularProgress /><p className="normalization-loading__title">{loadingJob ? 'Загружаем пакет' : 'Нормализуем адреса'}</p><p className="normalization-loading__hint">{loadingJob ? 'Восстанавливаем сохранённые результаты' : 'Результаты появятся здесь по мере готовности'}</p></div> : <p className="empty">{local && job ? 'В этой очереди записей нет.' : 'Загрузите файл или запустите пакет из поля выше.'}</p> : <div className="table-scroll" role="region" aria-label="Результаты проверки адресов" tabIndex={0}><p className="muted table-hint">Таблица прокручивается по горизонтали. Справа — статус и проверка.</p><table><thead><tr><th>Исходный адрес</th><th>Нормализованный адрес</th><th>Уверенность</th><th>Решение</th><th>Действие</th></tr></thead><tbody id="rows">{results.map((r, i) => reviewOnly && (r.status === 'matched' || isReviewed(r, job?.reviewed[i])) ? null : <Fragment key={i}><tr className={`${selected === i ? 'review-row--expanded ' : ''}${r.status !== 'matched' && !isReviewed(r, job?.reviewed[i]) ? 'requires-review' : ''}`}><td>{r.raw_address || <em>Пустая строка</em>}</td><td>{[r.city, r.street_type, r.street_name, r.house].filter(Boolean).join(', ') || 'Не определено'}</td><td>{confidenceLabel(r)}</td><td><ReviewStatus result={r} reviewed={job?.reviewed[i]} /></td><td><Button disabled={working} aria-expanded={selected === i} aria-controls={selected === i ? 'editor' : undefined} onClick={() => select(i)}>{selected === i ? 'Свернуть' : r.status === 'matched' || isReviewed(r, job?.reviewed[i]) ? 'Просмотреть' : 'Проверить'}</Button></td></tr>{selected === i && <tr className="review-details-row"><td colSpan={5}><section id="editor" className="panel editor" aria-label="Исправление адреса"><p className="address-caption">Исходный адрес</p><h3 id="correction-heading" className="raw-selected" tabIndex={-1}>{results[selected].raw_address || 'Пустая строка'}</h3><ResultWarnings result={results[selected]} />{job && <CityCandidatePicker job={job} index={selected} disabled={working || job.local?.run.status === 'published'} dirty={dirty} onBusy={setBusy} onSelected={saved => {
        const position=job.local?.rows[selected].position
        adopt(saved)
        const index=position===undefined ? selected : saved.local?.rows.findIndex(row=>row.position===position) ?? -1
        if(index<0 || !saved.results[index]) { setSelected(null); return }
        editResult(index,saved.results[index])
      }} />}<ResultDetails result={results[selected]} showWarnings={false} showStatus={false} /><div className="field-grid">{fieldNames.map(key => <TextField key={key} label={fieldLabels[key]} value={displayFieldValue(key, draft[key]) ?? ''} disabled={working || job?.local?.run.status === 'published'} onChange={e => { setDraft({ ...draft, [key]: e.target.value || null }); setDirty(true) }} />)}</div><div className="actions"><Button variant="filled" onClick={saveFields} disabled={working || job?.local?.run.status === 'published'}>Подтвердить адрес</Button></div></section></td></tr>}</Fragment>)}</tbody></table>{reviewOnly && !needsReview && <p className="empty">Все адреса обработаны. Очередь проверки пуста.</p>}</div>}
      {job?.local && <nav className="actions pagination" aria-label="Страницы результатов"><Button disabled={working || offset === 0} onClick={() => { if (leaveDraft()) { setDirty(false); setOffset(value => Math.max(0, value - 50)) } }}>Назад</Button><span>{results.length ? `Позиции в очереди: ${offset + 1}–${offset + results.length}` : 'Записей нет'}</span><Button disabled={working || results.length < 50} onClick={() => { if (leaveDraft()) { setDirty(false); setOffset(value => value + 50) } }}>Далее</Button></nav>}
    </section>}

    {job?.local?.run.source.kind === 'file' && <p id="file-output" className="muted">Правки сохраняются локально. Для предпросмотра и записи итогового файла или таблицы нажмите a в терминале jana normalize.</p>}
    {!local && <div className="workspace-methods"><LocalLaunch /></div>}

  </main>
}
