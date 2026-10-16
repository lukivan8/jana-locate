import type { Fields, Result } from '../../api/client'
export const fieldLabels: Record<keyof Fields, string> = { province: 'Область', city: 'Город', district: 'Район', living_area: 'Жилой массив', street_type: 'Тип улицы', street_name: 'Улица', house: 'Дом', apartment: 'Квартира', entrance: 'Подъезд', floor: 'Этаж', landmark: 'Ориентир' }
export const statusLabels = { matched: 'Найден в каталоге', needs_review: 'Нужна проверка', unmatched: 'Не распознан' }
const warningLabels: Record<string, string> = { libpostal_unavailable: 'libpostal недоступен: использован резервный разбор', gliner_unavailable: 'GLiNER недоступен: использован резервный разбор', coordinates_unknown: 'В каталоге нет координат здания', synthetic_catalog_coordinates: 'Синтетические координаты: требуют проверки', ambiguous_catalog_match: 'Несколько подходящих зданий', ambiguous_city_reference: 'Город неоднозначен в справочнике', ambiguous_reference_street: 'Улица неоднозначна в справочнике', building_reference_conflict: 'Здание противоречит справочнику', manually_reviewed: 'Проверено оператором' }
export function warningLabel(warning: string) {
  if (warningLabels[warning]) return warningLabels[warning]
  const field = warning.replace(/^(conflicting_|ambiguous_|ambiguous_reference_)/, '') as keyof Fields
  if (warning.startsWith('conflicting_')) return `Противоречивые данные: ${fieldLabels[field] ?? field}`
  if (warning.startsWith('ambiguous_')) return `Неоднозначность: ${fieldLabels[field] ?? field}`
  return warning
}
export function ResultDetails({ result }: { result: Result }) {
  return <div className="result-details"><div className="section-title"><h2>Нормализованный адрес</h2><span className={`badge ${result.status}`}>{statusLabels[result.status]}</span></div>
    <p className="normalized-address">{[result.city, result.street_type, result.street_name, result.house].filter(Boolean).join(', ') || 'Недостаточно данных для адреса'}</p>
    <dl className="parsed-fields">{Object.entries(fieldLabels).map(([key, label]) => <div key={key}><dt>{label}</dt><dd>{result[key as keyof Fields] ?? <span className="muted">Не определено</span>}</dd></div>)}</dl>
    <p className="muted">Уверенность: {Math.round(result.confidence * 100)}%. Оценка по признакам, не вероятность точности.</p>
    {!!result.warnings.length && <ul className="warning-list">{result.warnings.map((w, i) => <li key={i}>{warningLabel(w)}</li>)}</ul>}
    <details><summary>Ответ API · JSON</summary><pre>{JSON.stringify(result, null, 2)}</pre></details>
  </div>
}
