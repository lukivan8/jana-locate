import { useEffect, useState } from 'react'
import { cityCandidates, selectCity, type CityCandidates, type Job } from '../../api/client'
import { Button } from '../../ui/material'

export function CityCandidatePicker({ job, index, disabled, dirty, onSelected, onBusy }: {
  job: Job; index: number; disabled: boolean; dirty: boolean; onSelected: (saved: Job) => void; onBusy: (busy: boolean) => void
}) {
  const [data, setData] = useState<CityCandidates | null>(null)
  const [choice, setChoice] = useState('')
  const [editing, setEditing] = useState(false)
  const confirmed = job.city_selections?.find(c => c.record_index === index)
  const hasChoices = (data?.candidates.length ?? 0) > 1
  const selectedCandidate = data?.candidates.length === 1
    ? data.candidates[0]
    : data?.candidates.find(candidate => candidate.entity_id === confirmed?.entity_id)
  const [error, setError] = useState('')
  const [attempt, setAttempt] = useState(0)
  const [loading, setLoading] = useState(true)
  useEffect(() => {
    let active = true
    setLoading(true); setError(''); setData(null); setChoice(''); setEditing(false)
    cityCandidates(job, index).then(value => {
      if (!active) return
      setData(value)
      const previous = job.city_selections?.find(c => c.record_index === index && c.reference_version === value.reference_version)
      setChoice(previous?.entity_id ?? '')
    }).catch(e => { if (active) setError(e instanceof Error ? e.message : String(e)) })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [job.id, job.revision, index, attempt]) // A new revision must reload both candidates and their version.
  async function apply() {
    if (!data || !choice || data.revision !== job.revision) return
    onBusy(true); setError('')
    try { onSelected(await selectCity(job, index, data.revision, data.reference_version, choice)); setEditing(false) }
    catch (e) { setError(e instanceof Error ? e.message : String(e)) }
    finally { onBusy(false) }
  }
  if (!loading && !error && data?.candidates.length === 0) return null
  return <section className="city-candidates" aria-labelledby="city-candidates-heading" aria-busy={loading}>
    <h3 id="city-candidates-heading">Населённый пункт по справочнику</h3>
    {loading && <p role="status">Загружаем варианты…</p>}
    {error && <div role="alert" className="message error">{error}</div>}

    {selectedCandidate && !editing && <div className="city-selection-summary">
      <div><strong>{selectedCandidate.name} · {selectedCandidate.settlement_type ?? 'тип не указан'}</strong><span>{selectedCandidate.province ?? 'Область не указана'}</span><small>{selectedCandidate.source} · {selectedCandidate.entity_id}</small></div>
      {hasChoices && <Button variant="text" disabled={disabled || loading} aria-expanded={false} onClick={() => setEditing(true)}>Исправить</Button>}
    </div>}
    {data && hasChoices && (!selectedCandidate || editing) && <>
      <fieldset disabled={disabled || loading || dirty}>
        <legend>Варианты населённого пункта</legend>
        {data.candidates.map(candidate => <label className="city-candidate-option" key={candidate.entity_id}>
          <input type="radio" name="city-candidate" value={candidate.entity_id} checked={choice === candidate.entity_id} onChange={() => setChoice(candidate.entity_id)} />
          <span><strong>{candidate.name} · {candidate.settlement_type ?? 'тип не указан'}</strong><span>{candidate.province ?? 'Область не указана'}</span><small>{candidate.source} · {candidate.entity_id}</small></span>
        </label>)}
      </fieldset>
      {dirty && <p role="status">Сначала сохраните исправления формы или закройте её без сохранения и откройте запись заново.</p>}
      {data.revision !== job.revision && <p role="alert">Задание изменилось. Загрузите сохранённую версию перед выбором.</p>}
      <Button variant="filled" disabled={disabled || loading || dirty || !choice || data.revision !== job.revision} onClick={apply}>Выбрать населённый пункт</Button>
    </>}
    {(error || (!loading && data?.revision !== job.revision)) && <Button disabled={disabled || loading} onClick={() => setAttempt(value => value + 1)}>Обновить варианты</Button>}
  </section>
}
