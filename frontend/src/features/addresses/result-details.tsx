import type { Fields, Result } from '../../api/client'
export const fieldLabels: Record<keyof Fields, string> = { province: 'Область', city: 'Город', district: 'Район', living_area: 'Жилой массив', street_type: 'Тип улицы', street_name: 'Улица', house: 'Дом', apartment: 'Квартира', entrance: 'Подъезд', floor: 'Этаж', landmark: 'Ориентир' }
// Reference names can include the administrative type already shown by the field label.
export function displayFieldValue(field: keyof Fields, value: string | null | undefined) {
  if (field !== 'district' || !value) return value
  return value.replace(/^(?:район|р-н)\s+/iu, '').replace(/\s+(?:район|р-н|ауданы)$/iu, '').trim() || value
}
export const statusLabels = { matched: 'Нормализован', needs_review: 'Нужна проверка', unmatched: 'Не распознан' }
const warningLabels: Record<string, string> = { libpostal_unavailable: 'libpostal недоступен: использован резервный разбор', gliner_unavailable: 'Архивная диагностика: второй проход был недоступен', coordinates_unknown: 'В каталоге нет координат здания', synthetic_catalog_coordinates: 'Синтетические координаты: требуют проверки', ambiguous_catalog_match: 'Несколько подходящих зданий', city_selected_by_operator: 'Населённый пункт выбран оператором',
  district_inferred_from_address: 'Район определён по справочному адресу здания', district_inferred_from_street: 'Район определён по расположению улицы', district_inferred_from_kato: 'Район определён по справочнику КАТО', district_coverage_incomplete: 'Недостаточно справочных данных для однозначного определения района',
  ambiguous_city_reference: 'Город неоднозначен в справочнике', ambiguous_reference_street: 'Улица неоднозначна в справочнике', building_reference_conflict: 'Здание противоречит справочнику', manually_reviewed: 'Проверено оператором', manually_reviewed_fields: 'Адресные поля проверены оператором', building_fields_changed_coordinates_cleared: 'Поля здания изменены: прежняя геопозиция сброшена' }
export function warningLabel(warning: string) {
  if (warningLabels[warning]) return warningLabels[warning]
  const field = warning.replace(/^(conflicting_|ambiguous_|ambiguous_reference_)/, '') as keyof Fields
  if (warning.startsWith('conflicting_')) return `Противоречивые данные: ${fieldLabels[field] ?? field}`
  if (warning.startsWith('ambiguous_')) return `Неоднозначность: ${fieldLabels[field] ?? field}`
  return warning
}
export function missingCityReason(result: Result): string | null {
  if (result.city?.trim() || result.warnings.some(w => /^(ambiguous_|conflicting_)(city|city_reference)$/.test(w))) return null
  return 'Не указан или не распознан город (населённый пункт)'
}
export function isReviewed(result: Result, reviewed = false) {
  return reviewed || result.warnings.some(w => w === 'manually_reviewed' || w === 'manually_reviewed_fields')
}
export function ReviewStatus({ result, reviewed = false }: { result: Result; reviewed?: boolean }) {
  const manual = isReviewed(result, reviewed)
  const accepted = manual || result.status === 'matched'
  const label = manual ? 'Проверено оператором' : accepted ? 'Нормализован' : 'Требует проверки'
  return <span className={`review-status ${accepted ? 'review-status--accepted' : 'review-status--warning'}`} role="img" aria-label={label} title={label}>
    <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      {accepted ? <><circle cx="12" cy="12" r="9" /><path d="m8 12 3 3 5-6" /></> : <><path d="M10.3 4.5 2.7 18a2 2 0 0 0 1.7 3h15.2a2 2 0 0 0 1.7-3L13.7 4.5a2 2 0 0 0-3.4 0Z" /><path d="M12 9v4m0 4h.01" /></>}
    </svg>
  </span>
}
export function ResultWarnings({ result }: { result: Result }) {
  const cityReason = missingCityReason(result)
  const messages = [
    ...(cityReason ? [`${cityReason}. Уточните его в поле «Город».`] : []),
    ...(result.status !== 'matched' ? [`${statusLabels[result.status]}. Уверенность: ${Math.round(result.confidence * 100)}%. Проверьте адресные поля.`] : []),
    ...result.warnings.filter(w => !['manually_reviewed', 'manually_reviewed_fields', 'city_selected_by_operator'].includes(w)).map(warningLabel),
  ]
  if (!messages.length) return null
  return <ul className="review-warnings" aria-label="На что обратить внимание">{[...new Set(messages)].map(message => <li key={message}>
    <svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true"><path d="M12 8v5m0 4h.01" /><circle cx="12" cy="12" r="9" /></svg><span>{message}</span>
  </li>)}</ul>
}
export function ResultDetails({ result, showWarnings = true, showStatus = true }: { result: Result; showWarnings?: boolean; showStatus?: boolean }) {
  return <div className="result-details">
    {showWarnings && <ResultWarnings result={result} />}
    <div className="section-title"><h2 className="address-caption">Нормализованный адрес</h2>{showStatus && <ReviewStatus result={result} />}</div>
    <p className="normalized-address">{[result.city, result.street_type, result.street_name, result.house].filter(Boolean).join(', ') || 'Недостаточно данных для адреса'}</p>
    <dl className="parsed-fields">{Object.entries(fieldLabels).map(([key, label]) => <div key={key}><dt>{label}</dt><dd>{displayFieldValue(key as keyof Fields, result[key as keyof Fields]) ?? <span className="muted">Не определено</span>}</dd></div>)}</dl>
  </div>
}
