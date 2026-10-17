import type { LocalRun } from '../../api/client'

const number = new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 2 })

function duration(milliseconds: number) {
  if (milliseconds < 1000) return `${number.format(milliseconds)} мс`
  const seconds = Math.round(milliseconds / 1000)
  if (seconds < 60) return `${number.format(milliseconds / 1000)} с`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes} мин ${seconds % 60} с`
  return `${Math.floor(minutes / 60)} ч ${minutes % 60} мин ${seconds % 60} с`
}

export function RunTiming({ run }: { run: LocalRun }) {
  if (!run.normalization_completed_at || run.total <= 0 || (run.status !== 'review' && run.status !== 'published')) return null
  const elapsed = Date.parse(run.normalization_completed_at) - Date.parse(run.created_at)
  if (!Number.isFinite(elapsed) || elapsed < 0) return null
  return <p className="muted run-timing" title="От создания снимка до завершения обработки, включая ожидание и паузы. Среднее рассчитано по всем записям запуска.">Разбор: {duration(elapsed)} · В среднем: {duration(elapsed / run.total)} на запись</p>
}
