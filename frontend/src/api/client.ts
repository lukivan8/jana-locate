import createClient from 'openapi-fetch'
import type { components, paths } from './schema.gen'
export type Result = components['schemas']['Result']
export type Fields = components['schemas']['AddressFields']
export type InputRecord = components['schemas']['InputRecord']
export type CatalogEntry = components['schemas']['CatalogEntry']
declare global { interface Window { JANA_CONFIG?: { apiBaseUrl: string } } }
export function apiBaseUrl() {
  return (typeof window !== 'undefined' ? window.JANA_CONFIG?.apiBaseUrl : undefined) ?? import.meta.env.VITE_JANA_API_URL ?? 'http://localhost:8080'
}
function client() {
  return createClient<paths>({ baseUrl: apiBaseUrl(), fetch: request => fetch(request, { signal: AbortSignal.timeout(300_000) }) })
}
export async function normalize(records: InputRecord[]): Promise<Result[]> {
  const { data, error, response } = await client().POST('/v1/normalize', { body: { records } })
  if (!data) throw new Error(error?.message ?? `API returned ${response.status}`)
  return Array.isArray(data) ? data : data.results
}
export async function catalog(): Promise<CatalogEntry[]> {
  const { data, error, response } = await client().GET('/v1/catalog')
  if (!data) throw new Error(error?.message ?? `API returned ${response.status}`)
  return data.entries
}
export async function confirm(fields: Fields, latitude: number, longitude: number, expected_revision: number): Promise<CatalogEntry> {
  const { data, error, response } = await client().POST('/v1/catalog/confirmations', { body: { fields, coordinates: { latitude, longitude }, expected_revision } })
  if (!data) throw new Error(error?.message ?? `API returned ${response.status}`)
  return data
}
