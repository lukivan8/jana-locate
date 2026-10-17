import createClient from 'openapi-fetch'
import type { components, paths } from './schema.gen'
import type { components as LocalComponents, paths as LocalPaths } from './local-schema.gen'
export type Result = components['schemas']['Result']
export type Fields = components['schemas']['AddressFields']
export type InputRecord = components['schemas']['InputRecord']
export type CatalogEntry = components['schemas']['CatalogEntry']
declare global { interface Window { JANA_CONFIG?: { apiBaseUrl: string; workspace?: 'local' } } }
export function apiBaseUrl() {
  return (typeof window !== 'undefined' ? window.JANA_CONFIG?.apiBaseUrl : undefined) ?? import.meta.env.VITE_JANA_API_URL ?? 'http://localhost:8080'
}
async function apiFetch(request: Request): Promise<Response> {
  try { return await fetch(request, { signal: AbortSignal.timeout(300_000) }) }
  catch (error) {
    if (error instanceof DOMException && error.name === 'TimeoutError') throw new Error('Время ожидания ответа истекло. Повторите запрос.')
    throw new Error('Сервис недоступен. Проверьте подключение и адрес API, затем повторите запрос.')
  }
}
function client() {
  return createClient<paths>({ baseUrl: apiBaseUrl(), fetch: apiFetch })
}
export async function normalize(records: InputRecord[]): Promise<Result[]> {
  const { data, error, response } = await client().POST('/v1/normalize', { body: { records } })
  if (!data) throw new Error(error?.message ?? `Сервис вернул HTTP ${response.status}`)
  return Array.isArray(data) ? data : data.results
}
export async function catalog(): Promise<CatalogEntry[]> {
  const { data, error, response } = await client().GET('/v1/catalog')
  if (!data) throw new Error(error?.message ?? `Сервис вернул HTTP ${response.status}`)
  return data.entries
}
export async function confirm(fields: Fields, latitude: number, longitude: number, expected_revision: number): Promise<CatalogEntry> {
  const { data, error, response } = await client().POST('/v1/catalog/confirmations', { body: { fields, coordinates: { latitude, longitude }, expected_revision } })
  if (!data) throw new Error(error?.message ?? `Сервис вернул HTTP ${response.status}`)
  return data
}

export type LocalRun = LocalComponents['schemas']['Run']
export type LocalSource = LocalComponents['schemas']['ImportRequest']
export type Job = components['schemas']['NormalizationJob'] & { local?: { run: LocalRun; rows: LocalComponents['schemas']['Row'][]; offset: number; reviewOnly: boolean } }
export type JobSummary = components['schemas']['JobSummary'] & { sourceLabel?: string }
export async function createJob(records: InputRecord[]): Promise<Job> {
  if (isLocalWorkspace()) {
    const { data, error, response } = await localClient().POST('/v1/runs/input', { body: { records } })
    if (!data) throw new Error(error?.message ?? `Сервис вернул HTTP ${response.status}`)
    return getJob(data.id)
  }
  const { data, error, response } = await client().POST('/v1/normalization-jobs', { body: { records } })
  if (!data) throw new Error(error?.message ?? `Сервис вернул HTTP ${response.status}`)
  return data
}
export async function getJob(job_id: string, page = { offset: 0, reviewOnly: false }): Promise<Job> {
  if (isLocalWorkspace()) {
    const { data: run, error, response } = await localClient().GET('/v1/runs/{id}', { params: { path: { id: job_id } } })
    if (!run) throw new Error(error?.message ?? `Сервис вернул HTTP ${response.status}`)
    const { data, error: rowError, response: rowResponse } = await localClient().GET('/v1/runs/{id}/rows', { params: { path: { id: job_id }, query: { offset: page.offset, review_only: page.reviewOnly } } })
    if (!data) throw new Error(rowError?.message ?? `Сервис вернул HTTP ${rowResponse.status}`)
    const results = data.rows.map(row => ({ ...row.result, id: row.input_id ?? row.source_id, raw_address: row.raw_address ?? 'NULL' }))
    return { ...localSummary(run), city_selections: data.rows.flatMap((row,index) => row.city_selection ? [{...row.city_selection,record_index:index}] : []), records: results.map(r => ({ id: r.id, raw_address: r.raw_address })), results, reviewed: data.rows.map(row => row.reviewed), local: { run, rows: data.rows, offset: page.offset, reviewOnly: page.reviewOnly } }
  }
  const { data, error, response } = await client().GET('/v1/normalization-jobs/{job_id}', { params: { path: { job_id } } })
  if (!data) throw new Error(error?.message ?? `Сервис вернул HTTP ${response.status}`)
  return data
}
export async function listJobs(): Promise<JobSummary[]> {
  if (isLocalWorkspace()) {
    const { data, error, response } = await localClient().GET('/v1/runs')
    if (!data) throw new Error(error?.message ?? `Сервис вернул HTTP ${response.status}`)
    return data.runs.map(localSummary)
  }
  const { data, error, response } = await client().GET('/v1/normalization-jobs')
  if (!data) throw new Error(error?.message ?? `Сервис вернул HTTP ${response.status}`)
  return data.jobs
}
export async function reviewRecord(job: Job, record_index: number, fields: Fields, confirmed_catalog_id?: string): Promise<Job> {
  if (job.local) {
    const row = job.local.rows[record_index]
    const { data, error, response } = await localClient().POST('/v1/runs/{id}/rows/{position}/review', {
      params: { path: { id: job.id, position: row.position } }, body: { expected_revision: row.revision, fields },
    })
    if (!data) throw new Error(error?.message ?? `Сервис вернул HTTP ${response.status}`)
    return getJob(job.id, job.local)
  }
  const { data, error, response } = await client().POST('/v1/normalization-jobs/{job_id}/records/{record_index}/review', {
    params: { path: { job_id: job.id, record_index } }, body: { expected_revision: job.revision, fields, ...(confirmed_catalog_id ? { confirmed_catalog_id } : {}) },
  })
  if (!data) throw new Error(error?.message ?? `Сервис вернул HTTP ${response.status}`)
  return data
}
export async function deleteJob(job_id: string): Promise<void> {
  const { error, response } = await client().DELETE('/v1/normalization-jobs/{job_id}', { params: { path: { job_id } } })
  if (!response.ok) throw new Error(error?.message ?? `Сервис вернул HTTP ${response.status}`)
}

export type CityCandidates = components['schemas']['CityCandidates']
export async function cityCandidates(job: Job, record_index: number): Promise<CityCandidates> {
  if (job.local) {
    const {data,error,response}=await localClient().GET('/v1/runs/{id}/rows/{position}/candidates',{params:{path:{id:job.id,position:job.local.rows[record_index].position}}})
    if (!data) throw new Error(error?.message ?? `Сервис вернул HTTP ${response.status}`)
    return data
  }
  const job_id=job.id
  const { data, error, response } = await client().GET('/v1/normalization-jobs/{job_id}/records/{record_index}/candidates', { params: { path: { job_id, record_index } } })
  if (!data) throw new Error(error?.message ?? `Сервис вернул HTTP ${response.status}`)
  return data
}
export async function selectCity(job: Job, record_index: number, expected_revision: number, reference_version: string, entity_id: string): Promise<Job> {
  if (job.local) {
    const {data,error,response}=await localClient().POST('/v1/runs/{id}/rows/{position}/selections',{params:{path:{id:job.id,position:job.local.rows[record_index].position}},body:{expected_revision,reference_version,entity_id}})
    if (!data) throw new Error(error?.message ?? `Сервис вернул HTTP ${response.status}`)
    return getJob(job.id,job.local)
  }
  const job_id=job.id
  const { data, error, response } = await client().POST('/v1/normalization-jobs/{job_id}/records/{record_index}/selections', {
    params: { path: { job_id, record_index } }, body: { expected_revision, reference_version, entity_id },
  })
  if (!data) throw new Error(error?.message ?? `Сервис вернул HTTP ${response.status}`)
  return data
}


export function isLocalWorkspace() { return typeof window !== 'undefined' && window.JANA_CONFIG?.workspace === 'local' }
function localClient() { return createClient<LocalPaths>({ baseUrl: apiBaseUrl(), fetch: apiFetch }) }
function localSummary(run: LocalRun): JobSummary {
  return { id: run.id, revision: run.revision, status: run.status === 'normalizing' ? 'running' : run.status === 'failed' ? 'failed' : 'completed', created_at: run.created_at, updated_at: run.updated_at, expires_at: '', total: run.total, reviewed_count: run.reviewed, error: run.error, sourceLabel: run.source.kind === 'file' ? `Файл: ${run.source.file_name ?? run.source.table}` : `${run.source.schema}.${run.source.table}.${run.source.column}` }
}
export async function importLocalSource(source: LocalSource): Promise<string> {
  const { data, error, response } = await localClient().POST('/v1/runs', { body: source })
  if (!data) throw new Error(error?.message ?? `Сервис вернул HTTP ${response.status}`)
  return data.id
}
export async function localRunAction(job: Job, action: 'resume' | 'publish'): Promise<Job> {
  const { data, error, response } = await localClient().POST(action === 'resume' ? '/v1/runs/{id}/resume' : '/v1/runs/{id}/publish', { params: { path: { id: job.id } }, body: { expected_revision: job.revision } })
  if (!data) throw new Error(error?.message ?? `Сервис вернул HTTP ${response.status}`)
  return getJob(job.id, job.local)
}
export function localExportUrl(job: Job) { return `${apiBaseUrl()}/v1/runs/${job.id}/export` }

// The stream contains only journal metadata. Fetch the selected page through
// generated HTTP methods when its revision changes, never keep a raw-address cache.
export function subscribeLocalRuns(onRuns: (runs: LocalRun[]) => void, onConnection: (connected: boolean) => void): () => void {
  const path: keyof LocalPaths = '/v1/events'
  let stream: EventSource | null = null
  let disposed = false
  const receive = (event: MessageEvent<string>) => {
    try {
      const data = JSON.parse(event.data) as LocalComponents['schemas']['RunList']
      if (!Array.isArray(data.runs)) throw new Error('Некорректный снимок локального журнала')
      onRuns(data.runs); onConnection(true)
    } catch { onConnection(false) }
  }
  function disconnect() {
    if (!stream) return
    stream.removeEventListener('runs', receive as EventListener)
    stream.onopen = null; stream.onerror = null
    stream.close(); stream = null
    onConnection(false)
  }
  function syncVisibility() {
    if (disposed || document.visibilityState === 'hidden') { disconnect(); return }
    if (stream) return
    stream = new EventSource(`${apiBaseUrl()}${path}`)
    stream.onopen = () => onConnection(true)
    stream.onerror = () => onConnection(false)
    stream.addEventListener('runs', receive as EventListener)
  }
  // HTTP/1.1 limits connections per origin. Background tabs must not keep
  // permanent streams that can starve navigation and ordinary API requests.
  // Reconnecting sends a fresh journal snapshot, retaining editor drafts.
  document.addEventListener('visibilitychange', syncVisibility)
  window.addEventListener('pagehide', disconnect)
  window.addEventListener('pageshow', syncVisibility)
  syncVisibility()
  return () => {
    disposed = true
    document.removeEventListener('visibilitychange', syncVisibility)
    window.removeEventListener('pagehide', disconnect)
    window.removeEventListener('pageshow', syncVisibility)
    disconnect()
  }
}

export type ConnectionRequest = LocalComponents['schemas']['ConnectionRequest']
export type DatabaseCatalog = LocalComponents['schemas']['DatabaseCatalog']
export type RunSummary = LocalComponents['schemas']['RunSummary']
export type PublishedTable = LocalComponents['schemas']['PublishedTable']
export async function inspectDatabase(body: ConnectionRequest): Promise<DatabaseCatalog> {
  const { data, response } = await localClient().POST('/v1/connection/catalog', { body })
  if (!data) throw new Error(`Подключение не прошло (${response.status}). Проверьте доступ, хост, порт и пароль.`)
  return data
}
export async function saveConnection(body: ConnectionRequest) {
  const { data, response } = await localClient().POST('/v1/connection/save', { body })
  if (!data) throw new Error(`Не удалось сохранить источник (${response.status}). Проверьте права чтения, REFERENCES и создания таблицы, а также её имя.`)
  return data
}
export async function runSummary(id: string): Promise<RunSummary> {
  const { data, response } = await localClient().GET('/v1/runs/{id}/summary', { params: { path: { id } } })
  if (!data) throw new Error(`Не удалось загрузить сводку (${response.status}).`)
  return data
}
export async function publishedTable(id: string, offset: number): Promise<PublishedTable> {
  const { data, response } = await localClient().GET('/v1/runs/{id}/table', { params: { path: { id }, query: { offset } } })
  if (!data) throw new Error(`Не удалось прочитать записанную таблицу (${response.status}). Проверьте подключение CLI и права SELECT.`)
  return data
}

export type ConnectionProfile = LocalComponents['schemas']['ConnectionProfile']
export async function connectionProfile(): Promise<ConnectionProfile> {
  const { data, response } = await localClient().GET('/v1/connection')
  if (!data) throw new Error(`Не удалось загрузить сохранённое подключение (${response.status}). Повторите загрузку.`)
  return data
}
