import { useState } from 'react'
import spec from '../../api/openapi.json'
import specUrl from '../../api/openapi.json?url'
import { Button } from '../../ui/material'
import { apiBaseUrl } from '../../api/client'

type Schema = { $ref?: string; type?: string; nullable?: boolean; description?: string; properties?: Record<string, Schema>; required?: string[]; items?: Schema; enum?: string[]; oneOf?: Schema[]; minItems?: number; maxItems?: number }
type Operation = { operationId: string; requestBody?: { content: { 'application/json': { schema: Schema } } }; responses: Record<string, { description: string; content?: { 'application/json'?: { schema: Schema } } }> }
const descriptions: Record<string, { title: string; description: string }> = {
  '/v1/normalize': { title: 'Синхронный batch-запрос', description: 'Отправьте от 1 до 1 000 адресов в JSON. Результаты вернутся в ответе на этот же HTTP-запрос; создавать задание и опрашивать его статус не нужно.' },
  '/v1/normalize/candidates': { title: 'Варианты населённого пункта для записи', description: 'Принимает исходную запись и возвращает совместимые сущности и версию справочника. Использует тот же движок, что задания сайта. Подходит для CLI и собственного журнала; не создаёт задание и не сохраняет ответы.' },
  '/v1/normalize/selections': { title: 'Пересчёт с выбранным населённым пунктом', description: 'Пересчитывает исходную запись с явно выбранной сущностью. Подставьте entity_id и reference_version из ответа списка вариантов. Устаревшая версия отклоняется с 409. Выбор не подтверждает все поля адреса и не создаёт координаты или запись каталога.' },
  '/v1/catalog': { title: 'Каталог зданий', description: 'Устаревший отдельный каталог зданий. Не участвует в нормализации адресов.' },
  '/v1/catalog/confirmations': { title: 'Подтверждение здания', description: 'Устаревший отдельный метод: сохраняет проверенные поля здания и координаты, не влияя на нормализацию. expected_revision защищает от конкурентных изменений. Квартира, подъезд, этаж и ориентир не становятся свойствами здания.' },
  '/v1/normalization-jobs/{job_id}/records/{record_index}/candidates': { title: 'Варианты населённого пункта', description: 'Для выбранной позиции задания возвращает совместимые сущности справочника, их области и версию данных. Повторяющиеся пользовательские ID допустимы.' },
  '/v1/normalization-jobs/{job_id}/records/{record_index}/selections': { title: 'Выбор населённого пункта', description: 'Сохраняет выбранную сущность и пересчитывает запись из исходной строки. Заменяет ручные исправления этой записи. Геокодирование не выполняется. Защищён ревизией задания и версией справочника.' },
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
  '/v1/normalize/candidates': {record: {id:'demo-1',raw_address:'Караганда'}},
  '/v1/normalize/selections': {record: {id:'demo-1',raw_address:'Караганда'},entity_id:'<entity_id из списка>',reference_version:'<reference_version из ответа>'},
  '/v1/normalize': { records: [{ id: 'demo-1', raw_address: 'Ул .Курильская 9 кв 1' }, { id: 'demo-2', raw_address: 'Караганда' }] },
  '/v1/catalog/confirmations': { fields: { province: '<проверенная область>', city: '<проверенный населённый пункт>', district: null, living_area: null, street_type: '<тип улицы>', street_name: '<проверенное название>', house: '<номер дома>', apartment: null, entrance: null, floor: null, landmark: null }, coordinates: { latitude: '<число: проверенная широта>', longitude: '<число: проверенная долгота>' }, expected_revision: 0 },
}
const responses: Record<string, unknown> = {
  '/v1/normalize': { results: [{ ...exampleFields, id: 'demo-1', raw_address: 'Ул .Курильская 9 кв 1', confidence: 0.4, coordinates: null, catalog_id: null, catalog_revision: null, status: 'needs_review', warnings: [] }] },
}
export function ApiDocs() {
  const [selected, setSelected] = useState('POST /v1/normalize')
  const [copied, setCopied] = useState('')
  const endpoint = endpoints.find(e => `${e.method} ${e.path}` === selected)!
  const metadata = descriptions[endpoint.path] ?? { title: endpoint.operation.operationId, description: 'Описание метода в OpenAPI.' }
  const request = requests[endpoint.path]
  const curl = `curl -X ${endpoint.method} '${apiBaseUrl()}${endpoint.path}'${request ? ` \\\n  -H 'Content-Type: application/json' \\\n  -d '${JSON.stringify(request)}'` : ''}`
  async function copy(text: string) { try { await navigator.clipboard.writeText(text); setCopied('Запрос скопирован') } catch { setCopied('Не удалось скопировать. Выделите текст запроса вручную.') } }
  return <main id="main" className="api-main"><div className="intro"><div><h1>API нормализации адресов</h1><p>Отправьте пакет адресов и получите разобранные поля, статус и причины проверки.</p></div><a className="md-button md-button--outlined" href={specUrl} download="jana-openapi.json">Скачать OpenAPI</a></div>
    <div className="api-layout"><aside className="api-sidebar"><h2>HTTP API <span className="muted">{spec.info.version}</span></h2><nav aria-label="Основной метод API">{endpoints.filter(e => e.path === '/v1/normalize').map(e => <button type="button" key={`${e.method} ${e.path}`} aria-current={selected === `${e.method} ${e.path}` ? 'true' : undefined} onClick={() => { setSelected(`${e.method} ${e.path}`); setCopied('') }}><span className={`method ${e.method.toLowerCase()}`}>{e.method}</span><code>{e.path}</code></button>)}</nav><details open><summary>Другие методы</summary><nav aria-label="Другие методы API">{endpoints.filter(e => e.path !== '/v1/normalize').map(e => <button type="button" key={`${e.method} ${e.path}`} aria-current={selected === `${e.method} ${e.path}` ? 'true' : undefined} onClick={() => { setSelected(`${e.method} ${e.path}`); setCopied('') }}><span className={`method ${e.method.toLowerCase()}`}>{e.method}</span><code>{e.path}</code></button>)}</nav></details></aside>
    <article className="api-operation" key={selected}><div className="endpoint-title"><span className={`method ${endpoint.method.toLowerCase()}`}>{endpoint.method}</span><code>{endpoint.path}</code></div><h2>{metadata.title}</h2><p>{metadata.description}</p><p className="muted">Base URL: <code>{apiBaseUrl()}</code></p>
      {request !== undefined && <><div className="section-title"><h3>JSON запроса</h3><Button onClick={() => copy(JSON.stringify(request, null, 2))}>Копировать JSON</Button></div><p className="muted">Content-Type: <code>application/json</code></p><pre className="code-block">{JSON.stringify(request, null, 2)}</pre></>}{copied && <p role="status" className="muted">{copied}</p>}
      <details className="response-schema"><summary>Пример cURL</summary><div className="section-title"><h3>HTTP-запрос</h3><Button onClick={() => copy(curl)}>Копировать cURL</Button></div><pre className="code-block">{curl}</pre></details>
      {request !== undefined && <p className="muted">{endpoint.path === '/v1/catalog/confirmations' ? 'Шаблон требует подстановки проверенных данных. Замените координаты на числа. Заполненная команда записывает здание в каталог; без замены заполнителей API отклонит запрос.' : 'Данные примера синтетические.'}</p>}
      {endpoint.operation.requestBody && <details className="response-schema"><summary>Схема запроса · application/json</summary><SchemaTable schema={endpoint.operation.requestBody.content['application/json'].schema} /></details>}
      <details className="response-schema"><summary>Схемы ответов и коды ошибок</summary>{Object.entries(endpoint.operation.responses).map(([code, response]) => <details className="response-schema" key={code} ><summary><code>{code}</code> · {response.description}</summary>{response.content?.['application/json'] && <SchemaTable schema={response.content['application/json'].schema} />}</details>)}</details>
      {responses[endpoint.path] !== undefined && <details className="response-schema"><summary>Пример результата одной записи</summary><p className="muted">Пример формы ответа, не результат текущего запроса. Для пакета в results будет несколько записей.</p><pre className="code-block">{JSON.stringify(responses[endpoint.path], null, 2)}</pre></details>}
      <section className="contract-notes"><h3>Что возвращается в ответе</h3><ul><li>Порядок, id и исходная raw_address сохраняются.</li><li>Неизвестные адресные поля возвращаются как null; устаревшие поля координат всегда null.</li><li>matched означает полный разбор основных полей без неоднозначности; needs_review — неполный или противоречивый адрес; unmatched — адрес не распознан. warnings объясняет неопределённость.</li></ul></section>
    </article></div>
  </main>
}
