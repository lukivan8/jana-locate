import { useState } from 'react'
import spec from '../../api/openapi.json'
import specUrl from '../../api/openapi.json?url'
import { Button } from '../../ui/material'
import { apiBaseUrl } from '../../api/client'

type Schema = { $ref?: string; type?: string; nullable?: boolean; description?: string; properties?: Record<string, Schema>; required?: string[]; items?: Schema; enum?: string[]; oneOf?: Schema[]; minItems?: number; maxItems?: number }
type Operation = { operationId: string; requestBody?: { content: { 'application/json': { schema: Schema } } }; responses: Record<string, { description: string; content?: { 'application/json'?: { schema: Schema } } }> }
const descriptions: Record<string, { title: string; description: string }> = {
  '/v1/normalize': { title: 'Нормализация адресов', description: 'Один адрес или пакет до 1 000 записей. Возвращает разобранные поля, координаты известного здания, уверенность и причины проверки.' },
  '/v1/catalog': { title: 'Каталог зданий', description: 'Список зданий с координатами и ревизиями. Объединяет исходный каталог и подтверждённый оператором overlay.' },
  '/v1/catalog/confirmations': { title: 'Подтверждение здания', description: 'Сохраняет проверенные поля здания и координаты. expected_revision защищает от конкурентных изменений. Квартира, подъезд, этаж и ориентир не становятся свойствами здания.' },
  '/healthz': { title: 'Проверка процесса', description: 'Проверяет, что HTTP-процесс отвечает. Не доказывает доступность моделей.' },
  '/readyz': { title: 'Готовность сервиса', description: 'В текущей реализации возвращает тот же ответ, что /healthz. Доступность моделей проверяется при старте; деградация разбора отражается в warnings результата.' },
}
const endpoints = Object.entries(spec.paths).flatMap(([path, methods]) => Object.entries(methods).map(([method, operation]) => ({ path, method: method.toUpperCase(), operation: operation as Operation })))
function resolve(schema: Schema): Schema {
  if (!schema.$ref) return schema
  return (spec.components.schemas as Record<string, Schema>)[schema.$ref.split('/').at(-1)!] ?? schema
}
function schemaName(schema: Schema): string {
  if (schema.$ref) return schema.$ref.split('/').at(-1)!
  if (schema.oneOf) return schema.oneOf.map(schemaName).join(' | ')
  return `${schema.type ?? 'object'}${schema.type === 'array' && schema.items ? `<${schemaName(schema.items)}>` : ''}${schema.nullable ? ' | null' : ''}`
}
function SchemaTable({ schema }: { schema: Schema }) {
  const resolved = resolve(schema)
  if (resolved.oneOf) return <div>{resolved.oneOf.map((variant, i) => <div key={i}><h3>Вариант {i + 1} · {schemaName(variant)}</h3><SchemaTable schema={variant} /></div>)}</div>
  if (resolved.items) return <><p className="muted">Массив · {schemaName(resolved.items)}</p><SchemaTable schema={resolved.items} /></>
  if (!resolved.properties) return <pre>{JSON.stringify(resolved, null, 2)}</pre>
  return <div className="schema-scroll"><table className="schema-table"><thead><tr><th>Поле</th><th>Тип</th><th>Условия</th></tr></thead><tbody>{Object.entries(resolved.properties).map(([name, prop]) => <tr key={name}><td><code>{name}</code></td><td><code>{schemaName(prop)}</code></td><td>{resolved.required?.includes(name) ? 'Обязательное' : 'Опциональное'}{prop.minItems !== undefined && ` · ${prop.minItems}–${prop.maxItems} записей`}{prop.enum && ` · ${prop.enum.join(', ')}`}{prop.description && <small>{prop.description}</small>}{prop.$ref && resolve(prop).properties && <details><summary>Структура объекта</summary><SchemaTable schema={prop} /></details>}{prop.items && resolve(prop.items).type !== 'string' && <details><summary>Структура элемента</summary><SchemaTable schema={prop.items} /></details>}</td></tr>)}</tbody></table></div>
}
const exampleFields = { province: null, city: null, district: null, living_area: null, street_type: 'улица', street_name: 'Курильская', house: '9', apartment: '1', entrance: null, floor: null, landmark: null }
const requests: Record<string, unknown> = {
  '/v1/normalize': { records: [{ id: 'demo-1', raw_address: 'Ул .Курильская 9 кв 1' }] },
  '/v1/catalog/confirmations': { fields: { ...exampleFields, province: 'Тестовая область', city: 'Тестовый город', apartment: null }, coordinates: { latitude: 43.24, longitude: 76.89 }, expected_revision: 0 },
}
const responses: Record<string, unknown> = {
  '/v1/normalize': { results: [{ ...exampleFields, id: 'demo-1', raw_address: 'Ул .Курильская 9 кв 1', confidence: 0.4, coordinates: null, catalog_id: null, catalog_revision: null, status: 'needs_review', warnings: ['coordinates_unknown'] }] },
}
export function ApiDocs() {
  const [selected, setSelected] = useState('/v1/normalize')
  const [copied, setCopied] = useState('')
  const endpoint = endpoints.find(e => e.path === selected)!
  const metadata = descriptions[selected] ?? { title: endpoint.operation.operationId, description: 'Описание метода в OpenAPI.' }
  const request = requests[selected]
  const curl = `curl -X ${endpoint.method} '${apiBaseUrl()}${selected}'${request ? ` \\\n  -H 'Content-Type: application/json' \\\n  -d '${JSON.stringify(request)}'` : ''}`
  async function copy() { try { await navigator.clipboard.writeText(curl); setCopied('Запрос скопирован') } catch { setCopied('Не удалось скопировать. Выделите текст запроса вручную.') } }
  return <main id="main" className="api-main"><div className="intro"><div><h1>Контракт открыт. Границы определены.</h1><p>Независимый HTTP-сервис, статический фронтенд и внутренняя NLP-модель. Один OpenAPI-контракт связывает приложения через сгенерированные типы.</p></div><a className="md-button md-button--outlined" href={specUrl} download="jana-openapi.json">Скачать OpenAPI</a></div>
    <section className="architecture" aria-label="Архитектура контейнеров"><div><strong>Frontend / Nginx</strong><span>Статическая SPA · свой Dockerfile</span></div><span className="architecture-arrow">HTTP →</span><div><strong>Backend / Go</strong><span>Публичный API · свой Dockerfile</span></div><span className="architecture-arrow">internal →</span><div><strong>NLP / GLiNER</strong><span>CPU-модель · без публичного порта</span></div></section>
    <div className="api-layout"><aside className="api-sidebar"><h2>HTTP API <span className="muted">{spec.info.version}</span></h2><nav aria-label="Методы API">{endpoints.map(e => <button type="button" key={e.path} aria-current={selected === e.path ? 'true' : undefined} onClick={() => { setSelected(e.path); setCopied('') }}><span className={`method ${e.method.toLowerCase()}`}>{e.method}</span><code>{e.path}</code></button>)}</nav><details><summary>Структура и генерация</summary><pre>{`backend/\n  api/openapi.json   ← источник\n  pkg/api/           ← Go DTO\n  pkg/client/        ← Go SDK\n  internal/          ← реализация\nfrontend/\n  src/api/\n    openapi.json     ← snapshot\n    schema.gen.ts    ← TypeScript\nnlp/                 ← модель`}</pre><p className="muted">make generate обновляет Go DTO, snapshot и TypeScript. make check проверяет приложения и совпадение контрактов.</p></details></aside>
    <article className="api-operation" key={selected}><div className="endpoint-title"><span className={`method ${endpoint.method.toLowerCase()}`}>{endpoint.method}</span><code>{selected}</code></div><h2>{metadata.title}</h2><p>{metadata.description}</p><p className="muted">Base URL: <code>{apiBaseUrl()}</code></p>
      <div className="section-title"><h3>Пример запроса</h3><Button onClick={copy}>Копировать cURL</Button></div><pre className="code-block">{curl}</pre>{copied && <p role="status" className="muted">{copied}</p>}
      {request !== undefined && <p className="muted">Данные примера синтетические.{selected === '/v1/catalog/confirmations' && ' Эта команда записывает здание в каталог.'}</p>}
      {endpoint.operation.requestBody && <section><h3>Тело запроса · application/json</h3><SchemaTable schema={endpoint.operation.requestBody.content['application/json'].schema} /></section>}
      <section><h3>Ответы</h3>{Object.entries(endpoint.operation.responses).map(([code, response]) => <details className="response-schema" key={code} open={code === '200'}><summary><code>{code}</code> · {response.description}</summary>{response.content?.['application/json'] && <SchemaTable schema={response.content['application/json'].schema} />}</details>)}</section>
      {responses[selected] !== undefined && <section><h3>Иллюстрация ответа</h3><p className="muted">Пример формы ответа, не результат текущего запроса. Реальный разбор доступен на экранах обработки.</p><pre className="code-block">{JSON.stringify(responses[selected], null, 2)}</pre></section>}
      <section className="contract-notes"><h3>Гарантии и ограничения</h3><ul><li>Порядок, id и исходная raw_address сохраняются.</li><li>Неизвестные поля и координаты возвращаются как null.</li><li>matched, needs_review и unmatched описывают разные результаты. warnings объясняет неопределённость.</li><li>libpostal, правила и GLiNER работают локально. LLM-корректор в этот контракт не входит.</li><li>Справочники KATO/OSM неизменяемые. Подтверждения зданий пишутся в отдельный overlay.</li><li>Фронтенд собирается отдельно, получает только внешний URL API и не обращается к API при prerender.</li></ul></section>
    </article></div>
  </main>
}
